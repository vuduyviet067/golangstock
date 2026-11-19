import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { ApiError } from '../src/types/api';
import { RequestAbortedError } from '../src/services/dashboardApi';
import {
  dashboardRequestReducer,
  initialDashboardRequestState,
  type DashboardRequestAction,
  type DashboardRequestState,
} from '../src/hooks/useDashboardRequest';

type Data = { total: number };

const run = (...actions: DashboardRequestAction<Data>[]): DashboardRequestState<Data> =>
  actions.reduce(dashboardRequestReducer<Data>, initialDashboardRequestState<Data>());

describe('dashboardRequestReducer', () => {
  it('starts loading, without data or error', () => {
    assert.deepEqual(initialDashboardRequestState<Data>(), {
      requestId: 0,
      status: 'loading',
      data: null,
      error: null,
    });
  });

  it('stores the data of the current attempt', () => {
    assert.deepEqual(run({ type: 'start', requestId: 1 }, { type: 'success', requestId: 1, data: { total: 5 } }), {
      requestId: 1,
      status: 'success',
      data: { total: 5 },
      error: null,
    });
  });

  it('ignores the result of a superseded attempt', () => {
    const state = run(
      { type: 'start', requestId: 1 },
      { type: 'start', requestId: 2 },
      { type: 'success', requestId: 1, data: { total: 1 } },
      { type: 'failure', requestId: 1, error: new ApiError('x', 500) }
    );
    assert.deepEqual(state, { requestId: 2, status: 'loading', data: null, error: null });
  });

  it('classifies a failure', () => {
    const cases: [unknown, string][] = [
      [new ApiError('x', 0), 'network'],
      [new ApiError('x', 503), 'server'],
      [new ApiError('x', 401), 'unauthenticated'],
      [new ApiError('x', 403), 'forbidden'],
      [new ApiError('x', 400), 'invalid'],
      [new Error('x'), 'unexpected'],
    ];
    for (const [error, kind] of cases) {
      const state = run({ type: 'start', requestId: 1 }, { type: 'failure', requestId: 1, error });
      assert.equal(state.status, 'error');
      assert.equal(state.error, kind);
    }
  });

  it('leaves the state untouched on an abort', () => {
    const loading = run({ type: 'start', requestId: 3 });
    assert.equal(dashboardRequestReducer(loading, { type: 'failure', requestId: 3, error: new RequestAbortedError() }), loading);
    const done = run({ type: 'start', requestId: 1 }, { type: 'success', requestId: 1, data: { total: 2 } });
    assert.equal(dashboardRequestReducer(done, { type: 'failure', requestId: 1, error: new DOMException('a', 'AbortError') }), done);
  });

  it('a retry goes back to loading, clears the error and keeps the last data', () => {
    const state = run(
      { type: 'start', requestId: 1 },
      { type: 'success', requestId: 1, data: { total: 7 } },
      { type: 'start', requestId: 2 },
      { type: 'failure', requestId: 2, error: new ApiError('x', 0) },
      { type: 'start', requestId: 3 }
    );
    assert.deepEqual(state, { requestId: 3, status: 'loading', data: { total: 7 }, error: null });
  });

  it('a successful retry replaces the error', () => {
    const state = run(
      { type: 'start', requestId: 1 },
      { type: 'failure', requestId: 1, error: new ApiError('x', 502) },
      { type: 'start', requestId: 2 },
      { type: 'success', requestId: 2, data: { total: 9 } }
    );
    assert.deepEqual(state, { requestId: 2, status: 'success', data: { total: 9 }, error: null });
  });
});
