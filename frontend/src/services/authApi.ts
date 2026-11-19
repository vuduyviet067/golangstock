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
   * On success (200 OK), backend sets HttpOnly `access_token` and `refresh_token` cookies.
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
   * Get the current authenticated user by validating the access token cookie.
   * Returns 401 Unauthorized if not logged in or access token expired.
   */
  async getMe(): Promise<AuthUserResponse> {
    return request<AuthUserResponse>(`${AUTH_BASE_URL}/me`, {
      method: 'GET',
    });
  },

  /**
   * Refresh session: rotates the refresh token and sets new access and refresh cookies.
   */
  async refresh(): Promise<AuthUserResponse> {
    return request<AuthUserResponse>(`${AUTH_BASE_URL}/refresh`, {
      method: 'POST',
    });
  },

  /**
   * Log out the current user: revokes the refresh token and clears both cookies.
   */
  async logout(): Promise<LogoutResponse> {
    return request<LogoutResponse>(`${AUTH_BASE_URL}/logout`, {
      method: 'POST',
    });
  },
};
