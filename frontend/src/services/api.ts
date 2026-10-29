import { request } from './client';
import {
  CreateProductInput,
  Product,
  ProductListParams,
  UpdateProductInput,
} from '../types/product';

const BASE_URL = '/api/v1/products';

/**
 * Fetch products with optional query parameters.
 * Backend returns directly `Product[]`.
 */
export async function getProducts(params: ProductListParams = {}): Promise<Product[]> {
  const searchParams = new URLSearchParams();
  if (params && typeof params === 'object') {
    const allowedKeys: (keyof ProductListParams)[] = ['q', 'page', 'limit', 'sortBy', 'order'];
    for (const key of allowedKeys) {
      const val = params[key];
      if (val !== undefined && val !== null && String(val).trim() !== '') {
        searchParams.set(key, String(val));
      }
    }
  }

  const qs = searchParams.toString();
  const url = qs ? `${BASE_URL}?${qs}` : BASE_URL;

  return request<Product[]>(url, {
    method: 'GET',
  });
}

/**
 * Fetch a single product by ID.
 */
export async function getProduct(id: number | string): Promise<Product> {
  return request<Product>(`${BASE_URL}/${encodeURIComponent(id)}`, {
    method: 'GET',
  });
}

/**
 * Create a new product.
 */
export async function createProduct(data: CreateProductInput): Promise<Product> {
  return request<Product>(BASE_URL, {
    method: 'POST',
    body: data,
  });
}

/**
 * Update an existing product by ID.
 */
export async function updateProduct(id: number | string, data: UpdateProductInput): Promise<Product> {
  return request<Product>(`${BASE_URL}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: data,
  });
}

/**
 * Delete a product by ID (returns 204 No Content).
 */
export async function deleteProduct(id: number | string): Promise<null> {
  return request<null>(`${BASE_URL}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

/**
 * Increase stock for a product.
 */
export async function stockIn(id: number | string, quantity: number): Promise<Product> {
  return request<Product>(`${BASE_URL}/${encodeURIComponent(id)}/stock-in`, {
    method: 'POST',
    body: { quantity },
  });
}

/**
 * Decrease stock for a product.
 */
export async function stockOut(id: number | string, quantity: number): Promise<Product> {
  return request<Product>(`${BASE_URL}/${encodeURIComponent(id)}/stock-out`, {
    method: 'POST',
    body: { quantity },
  });
}
