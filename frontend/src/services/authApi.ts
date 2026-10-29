import { request } from './client';
import {
  AuthUserResponse,
  LoginRequest,
  LoginResponse,
  LogoutResponse,
  RegisterRequest,
} from '../types/auth';

const AUTH_BASE_URL = '/api/v1/auth';

export const authApi = {
  /**
   * Log in with email and password.
   * On success (200 OK), the backend sets the HttpOnly `session` cookie.
   */
  async login(data: LoginRequest): Promise<LoginResponse> {
    return request<LoginResponse>(`${AUTH_BASE_URL}/login`, {
      method: 'POST',
      body: data,
    });
  },

  /**
   * Register a new user account.
   */
  async register(data: RegisterRequest): Promise<AuthUserResponse> {
    return request<AuthUserResponse>(`${AUTH_BASE_URL}/register`, {
      method: 'POST',
      body: data,
    });
  },

  /**
   * Get the current authenticated user by validating the session cookie.
   * Returns 401 Unauthorized if not logged in or session expired.
   */
  async getMe(): Promise<AuthUserResponse> {
    return request<AuthUserResponse>(`${AUTH_BASE_URL}/me`, {
      method: 'GET',
    });
  },

  /**
   * Log out the current user: deletes the session record in DB and clears the cookie.
   */
  async logout(): Promise<LogoutResponse> {
    return request<LogoutResponse>(`${AUTH_BASE_URL}/logout`, {
      method: 'POST',
    });
  },
};
