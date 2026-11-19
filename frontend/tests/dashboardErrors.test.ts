import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { ApiError } from '../src/types/api';
import { RequestAbortedError } from '../src/services/dashboardApi';
import {
  DASHBOARD_ERROR_MESSAGES,
  classifyDashboardError,
  dashboardErrorAction,
  isRetryableDashboardError,
  type DashboardErrorKind,
} from '../src/utils/dashboardErrors';

describe('classifyDashboardError', () => {
  it('recognizes aborted requests', () => {
    assert.equal(classifyDashboardError(new RequestAbortedError()), 'aborted');
    assert.equal(classifyDashboardError(new DOMException('aborted', 'AbortError')), 'aborted');
  });

  it('treats ApiError status 0 as a network failure, not an abort', () => {
    assert.equal(classifyDashboardError(new ApiError('Network error', 0)), 'network');
    assert.equal(classifyDashboardError(new ApiError('aborted', 0, new DOMException('aborted', 'AbortError'))), 'network');
  });

  it('maps 401, 403 and 400', () => {
    assert.equal(classifyDashboardError(new ApiError('authentication required', 401)), 'unauthenticated');
    assert.equal(classifyDashboardError(new ApiError('forbidden: admin role required', 403)), 'forbidden');
    assert.equal(classifyDashboardError(new ApiError('invalid query parameters', 400)), 'invalid');
  });

  it('maps every 5xx to server', () => {
    for (const status of [500, 502, 503, 504, 599]) {
      assert.equal(classifyDashboardError(new ApiError('x', status)), 'server', String(status));
    }
  });

  it('maps other statuses and non-API errors to unexpected', () => {
    for (const status of [302, 404, 405, 409, 422, 429, 499, 600]) {
      assert.equal(classifyDashboardError(new ApiError('x', status)), 'unexpected', String(status));
    }
    assert.equal(classifyDashboardError(new TypeError('x is undefined')), 'unexpected');
    assert.equal(classifyDashboardError(new Error('x')), 'unexpected');
    assert.equal(classifyDashboardError(null), 'unexpected');
    assert.equal(classifyDashboardError(undefined), 'unexpected');
    assert.equal(classifyDashboardError('boom'), 'unexpected');
  });
});

const ALL_KINDS: DashboardErrorKind[] = [
  'aborted',
  'unauthenticated',
  'forbidden',
  'invalid',
  'network',
  'server',
  'unexpected',
];

describe('retry policy', () => {
  it('retries only network failures and 5xx', () => {
    const retryable = ALL_KINDS.filter(isRetryableDashboardError);
    assert.deepEqual(retryable, ['network', 'server']);
  });

  it('never retries an abort, 400, 401 or 403', () => {
    for (const kind of ['aborted', 'invalid', 'unauthenticated', 'forbidden'] as const) {
      assert.equal(isRetryableDashboardError(kind), false, kind);
      assert.notEqual(dashboardErrorAction(kind), 'retry', kind);
    }
  });

  it('offers a session check for 401 and 403, nothing for the rest', () => {
    assert.deepEqual(
      Object.fromEntries(ALL_KINDS.map((kind) => [kind, dashboardErrorAction(kind)])),
      {
        aborted: 'none',
        unauthenticated: 'reauthenticate',
        forbidden: 'reauthenticate',
        invalid: 'none',
        network: 'retry',
        server: 'retry',
        unexpected: 'none',
      }
    );
  });
});

describe('DASHBOARD_ERROR_MESSAGES', () => {
  it('has a message for every kind except aborted', () => {
    assert.deepEqual(
      Object.keys(DASHBOARD_ERROR_MESSAGES).sort(),
      ALL_KINDS.filter((kind) => kind !== 'aborted').sort()
    );
  });

  it('has an action label exactly when there is an action', () => {
    for (const [kind, message] of Object.entries(DASHBOARD_ERROR_MESSAGES)) {
      assert.ok(message.title.trim() && message.description.trim(), kind);
      const hasAction = dashboardErrorAction(kind as DashboardErrorKind) !== 'none';
      assert.equal(Boolean(message.actionLabel), hasAction, kind);
    }
  });

  it('labels the actions in Vietnamese', () => {
    assert.equal(DASHBOARD_ERROR_MESSAGES.network.actionLabel, 'Thử lại');
    assert.equal(DASHBOARD_ERROR_MESSAGES.server.actionLabel, 'Thử lại');
    assert.equal(DASHBOARD_ERROR_MESSAGES.unauthenticated.actionLabel, 'Đăng nhập lại');
    assert.equal(DASHBOARD_ERROR_MESSAGES.forbidden.actionLabel, 'Cập nhật quyền');
  });
});
