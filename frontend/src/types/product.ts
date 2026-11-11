export interface Product {
  id: number;
  sku: string;
  name: string;
  price: number;
  quantity: number;
  // Low-stock threshold: a product is low on stock when 0 < quantity <= minimum_stock.
  minimum_stock: number;
}

export interface ProductListParams {
  q?: string;
  page?: number | string;
  limit?: number | string;
  sortBy?: 'id' | 'name' | 'price' | string;
  order?: 'asc' | 'desc' | string;
}

export interface CreateProductInput {
  sku: string;
  name: string;
  price: number;
  quantity: number;
  // Optional; the API defaults it to 0.
  minimum_stock?: number;
}

// quantity is intentionally absent: stock changes only via stock-in/stock-out,
// and the API rejects a PUT body that contains quantity.
export interface UpdateProductInput {
  sku: string;
  name: string;
  price: number;
  // Optional; when omitted the API keeps the current value.
  minimum_stock?: number;
}

export interface StockAdjustmentInput {
  quantity: number;
}
