const BASE_URL = '/api/v1/products'

/**
 * Helper to execute HTTP requests with robust error handling.
 */
async function request(url, options = {}) {
  let response
  try {
    response = await fetch(url, options)
  } catch (err) {
    const networkError = new Error(
      err?.message || 'Network error: Failed to connect to server'
    )
    networkError.cause = err
    throw networkError
  }

  // 204 No Content indicates success with no response body
  if (response.status === 204) {
    return null
  }

  const contentType = response.headers.get('content-type') || ''
  const isJson = contentType.includes('application/json')

  let data = null
  let text = null

  try {
    if (isJson) {
      data = await response.json()
    } else {
      text = await response.text()
    }
  } catch {
    // If parsing fails, proceed with null data/text
  }

  if (!response.ok) {
    let message = ''
    if (data && typeof data === 'object' && typeof data.error === 'string') {
      message = data.error
    } else if (text && text.trim()) {
      message = text.trim()
    } else {
      message = response.statusText || `Request failed with status ${response.status}`
    }

    const error = new Error(message)
    error.status = response.status
    error.data = data
    throw error
  }

  return data ?? text
}

/**
 * Fetch products with optional query parameters.
 * Supported params: q, page, limit, sortBy, order
 *
 * @param {Object} [params]
 * @param {string} [params.q]
 * @param {number|string} [params.page]
 * @param {number|string} [params.limit]
 * @param {string} [params.sortBy] - 'id' | 'name' | 'price'
 * @param {string} [params.order] - 'asc' | 'desc'
 * @returns {Promise<Array>}
 */
export async function getProducts(params = {}) {
  const searchParams = new URLSearchParams()
  if (params && typeof params === 'object') {
    const allowedKeys = ['q', 'page', 'limit', 'sortBy', 'order']
    for (const key of allowedKeys) {
      const val = params[key]
      if (val !== undefined && val !== null && String(val).trim() !== '') {
        searchParams.set(key, String(val))
      }
    }
  }

  const qs = searchParams.toString()
  const url = qs ? `${BASE_URL}?${qs}` : BASE_URL

  return request(url, {
    method: 'GET',
  })
}

/**
 * Fetch a single product by ID.
 *
 * @param {number|string} id
 * @returns {Promise<Object>}
 */
export async function getProduct(id) {
  return request(`${BASE_URL}/${encodeURIComponent(id)}`, {
    method: 'GET',
  })
}

/**
 * Create a new product.
 *
 * @param {Object} data - { sku, name, price, quantity }
 * @returns {Promise<Object>}
 */
export async function createProduct(data) {
  return request(BASE_URL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(data),
  })
}

/**
 * Update an existing product by ID.
 *
 * @param {number|string} id
 * @param {Object} data - { sku, name, price, quantity }
 * @returns {Promise<Object>}
 */
export async function updateProduct(id, data) {
  return request(`${BASE_URL}/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(data),
  })
}

/**
 * Delete a product by ID.
 *
 * @param {number|string} id
 * @returns {Promise<null>}
 */
export async function deleteProduct(id) {
  return request(`${BASE_URL}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

/**
 * Increase stock for a product.
 *
 * @param {number|string} id
 * @param {number} quantity - Positive integer
 * @returns {Promise<Object>}
 */
export async function stockIn(id, quantity) {
  return request(`${BASE_URL}/${encodeURIComponent(id)}/stock-in`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ quantity }),
  })
}

/**
 * Decrease stock for a product.
 *
 * @param {number|string} id
 * @param {number} quantity - Positive integer
 * @returns {Promise<Object>}
 */
export async function stockOut(id, quantity) {
  return request(`${BASE_URL}/${encodeURIComponent(id)}/stock-out`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ quantity }),
  })
}
