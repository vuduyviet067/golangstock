import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { User, LoginRequest, RegisterRequest } from '../types/auth';
import { authApi } from '../services/authApi';
import { ApiError } from '../types/api';

export interface AuthContextType {
  user: User | null;
  loading: boolean;
  login: (credentials: LoginRequest) => Promise<void>;
  register: (credentials: RegisterRequest) => Promise<void>;
  logout: () => Promise<void>;
  checkAuth: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState<boolean>(true);

  /**
   * Check whether the user currently holds an active session.
   * Requirement 6: HTTP 401 is an expected unauthenticated state, not an application error.
   */
  const checkAuth = useCallback(async () => {
    try {
      setLoading(true);
      const currentUser = await authApi.getMe();
      setUser(currentUser);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setUser(null);
      } else {
        // Any other error (e.g. backend disconnected) also defaults user to null
        setUser(null);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  /**
   * Log in user using email and password.
   * On success, backend sets the HttpOnly session cookie.
   * We then retrieve the authenticated user profile.
   */
  const login = async (credentials: LoginRequest) => {
    await authApi.login(credentials);
    const currentUser = await authApi.getMe();
    setUser(currentUser);
  };

  /**
   * Register a new user account.
   */
  const register = async (credentials: RegisterRequest) => {
    await authApi.register(credentials);
  };

  /**
   * Log out user: calls POST /api/v1/auth/logout to invalidate session on server
   * and clears frontend authentication state.
   */
  const logout = async () => {
    try {
      await authApi.logout();
    } finally {
      setUser(null);
    }
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, register, logout, checkAuth }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
