import { ApiError, ApiErrorResponse } from '../types/api';

export interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
}

/**
 * Robust HTTP client using native fetch.
 * Explicitly sends `credentials: 'include'` to ensure HttpOnly session cookies
 * are transmitted and preserved across all requests.
 */
export async function request<T>(url: string, options: RequestOptions = {}): Promise<T> {
  const { body, headers, ...customOptions } = options;

  const isPlainObjectOrArray =
    body !== undefined &&
    body !== null &&
    typeof body === 'object' &&
    !(body instanceof FormData) &&
    !(body instanceof Blob);

  const config: RequestInit = {
    ...customOptions,
    credentials: 'include', // Support HttpOnly session cookies
    headers: {
      Accept: 'application/json',
      ...(isPlainObjectOrArray ? { 'Content-Type': 'application/json' } : {}),
      ...headers,
    },
  };

  if (body !== undefined) {
    config.body = isPlainObjectOrArray ? JSON.stringify(body) : (body as BodyInit);
  }

  let response: Response;
  try {
    response = await fetch(url, config);
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : 'Network error: Failed to connect to server';
    throw new ApiError(errorMsg, 0, err);
  }

  // 204 No Content indicates success with no response body
  if (response.status === 204) {
    return null as unknown as T;
  }

  const contentType = response.headers.get('content-type') || '';
  const isJson = contentType.includes('application/json');

  let data: unknown = null;
  let text = '';

  try {
    if (isJson) {
      data = await response.json();
    } else {
      text = await response.text();
    }
  } catch {
    // Body parsing fallback
  }

  if (!response.ok) {
    let message = '';
    if (data && typeof data === 'object') {
      const errObj = data as ApiErrorResponse;
      if (typeof errObj.error === 'string') {
        message = errObj.error;
      } else if (typeof errObj.message === 'string') {
        message = errObj.message;
      }
    }

    if (!message && text.trim()) {
      message = text.trim();
    }

    if (!message) {
      message = response.statusText || `Request failed with status ${response.status}`;
    }

    throw new ApiError(message, response.status, data);
  }

  return (isJson ? data : text) as T;
}
