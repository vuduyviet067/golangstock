import { useState, type FormEvent } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { ApiError } from '../types/api';

export function LoginView() {
  const { login, register } = useAuth();
  const [isRegisterMode, setIsRegisterMode] = useState(false);

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  const [fieldErrors, setFieldErrors] = useState<{ email?: string; password?: string }>({});
  const [apiError, setApiError] = useState<string | null>(null);
  const [successNotice, setSuccessNotice] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const validate = (): boolean => {
    const errors: { email?: string; password?: string } = {};

    const cleanEmail = email.trim();
    if (!cleanEmail) {
      errors.email = 'Vui lòng nhập địa chỉ email';
    } else {
      // Basic email format validation (matches backend isValidEmail rule)
      const atIdx = cleanEmail.indexOf('@');
      const hasDot = atIdx > 0 && cleanEmail.slice(atIdx + 1).includes('.');
      const hasSpace = /\s/.test(cleanEmail);
      if (atIdx <= 0 || atIdx === cleanEmail.length - 1 || !hasDot || hasSpace) {
        errors.email = 'Email không hợp lệ (ví dụ: user@example.com)';
      }
    }

    if (!password) {
      errors.password = 'Vui lòng nhập mật khẩu';
    } else if (password.length < 8) {
      errors.password = 'Mật khẩu phải có ít nhất 8 ký tự';
    } else if (password.length > 72) {
      errors.password = 'Mật khẩu tối đa 72 ký tự';
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setApiError(null);
    setSuccessNotice(null);

    if (!validate()) {
      return;
    }

    setIsSubmitting(true);

    try {
      if (isRegisterMode) {
        // Register flow
        await register({ email: email.trim(), password });
        setSuccessNotice('Đăng ký tài khoản thành công! Đang tự động đăng nhập...');
        // Auto-login right after registration
        await login({ email: email.trim(), password });
      } else {
        // Login flow
        await login({ email: email.trim(), password });
      }
      // Successful login automatically navigates via AuthContext user state change
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          setApiError('Email hoặc mật khẩu không chính xác.');
        } else if (err.status === 409) {
          setApiError('Email này đã được sử dụng. Vui lòng chọn email khác hoặc đăng nhập.');
        } else {
          setApiError(err.message || 'Yêu cầu không thành công.');
        }
      } else if (err instanceof Error) {
        setApiError(err.message);
      } else {
        setApiError('Đã xảy ra lỗi không xác định. Vui lòng thử lại.');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const toggleMode = () => {
    setIsRegisterMode((prev) => !prev);
    setFieldErrors({});
    setApiError(null);
    setSuccessNotice(null);
  };

  return (
    <div className="auth-wrapper">
      <div className="auth-card">
        {/* Brand header */}
        <div className="auth-header">
          <div className="brand-logo" aria-hidden="true">
            📦
          </div>
          <h1 className="brand-title">GoStock</h1>
          <p className="auth-subtitle">
            {isRegisterMode ? 'Tạo tài khoản quản trị kho mới' : 'Đăng nhập vào hệ thống quản lý kho'}
          </p>
        </div>

        {/* Mode switch tabs */}
        <div className="auth-tabs" role="tablist">
          <button
            type="button"
            className={`auth-tab ${!isRegisterMode ? 'active' : ''}`}
            onClick={() => !isSubmitting && isRegisterMode && toggleMode()}
            disabled={isSubmitting}
            role="tab"
            aria-selected={!isRegisterMode}
          >
            Đăng nhập
          </button>
          <button
            type="button"
            className={`auth-tab ${isRegisterMode ? 'active' : ''}`}
            onClick={() => !isSubmitting && !isRegisterMode && toggleMode()}
            disabled={isSubmitting}
            role="tab"
            aria-selected={isRegisterMode}
          >
            Đăng ký
          </button>
        </div>

        {/* Error / Success Notice */}
        {apiError && (
          <div className="alert alert-danger" role="alert" style={{ marginBottom: 16 }}>
            <span className="alert-icon" aria-hidden="true">⚠</span>
            <span>{apiError}</span>
          </div>
        )}

        {successNotice && (
          <div className="alert alert-success" role="status" style={{ marginBottom: 16 }}>
            <span className="alert-icon" aria-hidden="true">✓</span>
            <span>{successNotice}</span>
          </div>
        )}

        {/* Login / Register Form */}
        <form onSubmit={handleSubmit} className="auth-form" noValidate>
          <div className="form-group">
            <label className="form-label" htmlFor="auth-email">
              Email <span className="form-required">*</span>
            </label>
            <input
              id="auth-email"
              type="email"
              autoComplete="email"
              className={`form-input ${fieldErrors.email ? 'has-error' : ''}`}
              placeholder="user@example.com"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value);
                if (fieldErrors.email) setFieldErrors((prev) => ({ ...prev, email: undefined }));
                if (apiError) setApiError(null);
              }}
              disabled={isSubmitting}
            />
            {fieldErrors.email && <span className="field-error">{fieldErrors.email}</span>}
          </div>

          <div className="form-group">
            <label className="form-label" htmlFor="auth-password">
              Mật khẩu <span className="form-required">*</span>
            </label>
            <input
              id="auth-password"
              type="password"
              autoComplete={isRegisterMode ? 'new-password' : 'current-password'}
              className={`form-input ${fieldErrors.password ? 'has-error' : ''}`}
              placeholder="••••••••"
              value={password}
              onChange={(e) => {
                setPassword(e.target.value);
                if (fieldErrors.password) setFieldErrors((prev) => ({ ...prev, password: undefined }));
                if (apiError) setApiError(null);
              }}
              disabled={isSubmitting}
            />
            {fieldErrors.password && <span className="field-error">{fieldErrors.password}</span>}
            {isRegisterMode && (
              <span className="form-hint">Mật khẩu có độ dài từ 8 đến 72 ký tự</span>
            )}
          </div>

          <button
            type="submit"
            className="btn btn-primary auth-submit-btn"
            disabled={isSubmitting}
          >
            {isSubmitting ? (
              <>
                <span className="spinner" aria-hidden="true"></span>
                <span>{isRegisterMode ? 'Đang tạo tài khoản...' : 'Đang xác thực...'}</span>
              </>
            ) : (
              <span>{isRegisterMode ? 'Tạo tài khoản' : 'Đăng nhập'}</span>
            )}
          </button>
        </form>

        <div className="auth-footer-help">
          <span>
            {isRegisterMode ? 'Đã có tài khoản?' : 'Chưa có tài khoản?'}
          </span>{' '}
          <button
            type="button"
            className="auth-link-btn"
            onClick={toggleMode}
            disabled={isSubmitting}
          >
            {isRegisterMode ? 'Đăng nhập ngay' : 'Đăng ký tài khoản'}
          </button>
        </div>
      </div>
    </div>
  );
}
