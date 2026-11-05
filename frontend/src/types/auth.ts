export interface User {
  id: number;
  email: string;
  role: 'user' | 'admin' | string;
  created_at: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  id: number;
  email: string;
  role: string;
  created_at: string;
}

export interface AuthUserResponse {
  id: number;
  email: string;
  role: string;
  created_at: string;
}

export interface LogoutResponse {
  message: string;
}
