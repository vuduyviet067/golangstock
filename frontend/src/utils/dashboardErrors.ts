import { ApiError } from '../types/api';
import { isAbortError } from '../services/dashboardApi';

// How the dashboard reacts to a failed request. Only network failures and
// 5xx responses may be retried, and only when the admin asks. 401 and 403
// need the session or the role to be checked again (the client already
// tried its silent refresh), 400 and other statuses will not change on a
// retry, and an aborted request was cancelled on purpose.

export type DashboardErrorKind =
  | 'aborted'
  | 'unauthenticated'
  | 'forbidden'
  | 'invalid'
  | 'network'
  | 'server'
  | 'unexpected';

export type DashboardErrorAction = 'retry' | 'reauthenticate' | 'none';

export function classifyDashboardError(err: unknown): DashboardErrorKind {
  if (isAbortError(err)) {
    return 'aborted';
  }
  if (!(err instanceof ApiError)) {
    return 'unexpected';
  }
  if (err.status === 0) {
    return 'network';
  }
  if (err.status === 401) {
    return 'unauthenticated';
  }
  if (err.status === 403) {
    return 'forbidden';
  }
  if (err.status === 400) {
    return 'invalid';
  }
  if (err.status >= 500 && err.status <= 599) {
    return 'server';
  }
  return 'unexpected';
}

export function isRetryableDashboardError(kind: DashboardErrorKind): boolean {
  return kind === 'network' || kind === 'server';
}

/** What the error notice offers: retry, a session/role check, or nothing. */
export function dashboardErrorAction(kind: DashboardErrorKind): DashboardErrorAction {
  if (isRetryableDashboardError(kind)) {
    return 'retry';
  }
  if (kind === 'unauthenticated' || kind === 'forbidden') {
    return 'reauthenticate';
  }
  return 'none';
}

export interface DashboardErrorMessage {
  title: string;
  description: string;
  // Label of the action button, when there is one.
  actionLabel?: string;
}

// No message for 'aborted': a cancelled request is never shown.
export const DASHBOARD_ERROR_MESSAGES: Readonly<Record<Exclude<DashboardErrorKind, 'aborted'>, DashboardErrorMessage>> = {
  unauthenticated: {
    title: 'Phiên đăng nhập đã hết hạn',
    description: 'Vui lòng đăng nhập lại để tiếp tục xem Dashboard.',
    actionLabel: 'Đăng nhập lại',
  },
  forbidden: {
    title: 'Không có quyền xem Dashboard',
    description: 'Quyền của tài khoản có thể đã thay đổi. Cập nhật quyền để tiếp tục.',
    actionLabel: 'Cập nhật quyền',
  },
  invalid: {
    title: 'Yêu cầu không hợp lệ',
    description: 'Máy chủ từ chối yêu cầu Dashboard.',
  },
  network: {
    title: 'Không thể kết nối tới máy chủ',
    description: 'Kiểm tra kết nối mạng rồi thử lại.',
    actionLabel: 'Thử lại',
  },
  server: {
    title: 'Máy chủ đang gặp lỗi',
    description: 'Máy chủ chưa xử lý được yêu cầu. Vui lòng thử lại sau ít phút.',
    actionLabel: 'Thử lại',
  },
  unexpected: {
    title: 'Đã xảy ra lỗi không mong muốn',
    description: 'Không thể tải dữ liệu Dashboard. Hãy tải lại trang nếu lỗi tiếp diễn.',
  },
};
