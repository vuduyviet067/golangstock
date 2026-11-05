import { ApiError, ApiErrorResponse, ApiResponse } from '../types/api';

export interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
}

// Singleton Promise to synchronize concurrent refresh requests and avoid race conditions
let refreshPromise: Promise<boolean> | null = null;

async function doRefresh(): Promise<boolean> {
  if (refreshPromise) {
    return refreshPromise;
  }

  refreshPromise = (async () => {
    try {
      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        credentials: 'include',
        headers: {
          Accept: 'application/json',
        },
      });
      return res.ok;
    } catch {
      return false;
    } finally {
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

/**
 * Robust HTTP client using native fetch.
 * Explicitly sends `credentials: 'include'` to ensure HttpOnly authentication cookies
 * are transmitted and preserved across all requests.
 * Automatically attempts a silent refresh and retry when receiving HTTP 401 on protected resources.
 */
export async function request<T>(
  url: string,
  options: RequestOptions = {},
  isRetry = false
): Promise<T> {
  const { body, headers, ...customOptions } = options;

  const isPlainObjectOrArray =
    body !== undefined &&
    body !== null &&
    typeof body === 'object' &&
    !(body instanceof FormData) &&
    !(body instanceof Blob);

  const config: RequestInit = {
    ...customOptions,
    credentials: 'include', // Support HttpOnly access and refresh cookies
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

  // Intercept 401 Unauthorized for automatic silent token refresh and retry
  if (response.status === 401 && !isRetry) {
    const isAuthRoute =
      url.includes('/api/v1/auth/login') ||
      url.includes('/api/v1/auth/register') ||
      url.includes('/api/v1/auth/refresh');

    if (!isAuthRoute) {
      const refreshed = await doRefresh();
      if (refreshed) {
        // Retry the original request with new access token cookie
        return request<T>(url, options, true);
      }
    }
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

  if (isJson && data !== null && typeof data === 'object' && 'success' in data && 'data' in data) {
    const enveloped = data as ApiResponse<T>;
    return enveloped.data;
  }

  return (isJson ? data : text) as T;
}
