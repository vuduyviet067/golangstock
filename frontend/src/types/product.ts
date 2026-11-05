export interface Product {
  id: number;
  sku: string;
  name: string;
  price: number;
  quantity: number;
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
}

export interface UpdateProductInput {
  sku: string;
  name: string;
  price: number;
  quantity: number;
}

export interface StockAdjustmentInput {
  quantity: number;
}
