import { useCallback, useEffect, useReducer, useRef, useState } from 'react';
import { classifyDashboardError, type DashboardErrorKind } from '../utils/dashboardErrors';

// Loads dashboard data with one AbortController per attempt. A new attempt
// (retry) or unmounting aborts the previous one, and the result of an
// aborted or superseded attempt never reaches the state. Retrying is
// manual: nothing here retries on its own.

export interface DashboardRequestState<T> {
  // Attempt the state belongs to; results of other attempts are ignored.
  requestId: number;
  status: 'loading' | 'success' | 'error';
  // Data of the last successful attempt, kept while reloading or after an error.
  data: T | null;
  // Set when status is 'error'; never 'aborted'.
  error: DashboardErrorKind | null;
}

export type DashboardRequestAction<T> =
  | { type: 'start'; requestId: number }
  | { type: 'success'; requestId: number; data: T }
  | { type: 'failure'; requestId: number; error: unknown };

export function initialDashboardRequestState<T>(): DashboardRequestState<T> {
  return { requestId: 0, status: 'loading', data: null, error: null };
}

export function dashboardRequestReducer<T>(
  state: DashboardRequestState<T>,
  action: DashboardRequestAction<T>
): DashboardRequestState<T> {
  switch (action.type) {
    case 'start':
      return { requestId: action.requestId, status: 'loading', data: state.data, error: null };
    case 'success':
      if (action.requestId !== state.requestId) {
        return state;
      }
      return { ...state, status: 'success', data: action.data, error: null };
    case 'failure': {
      if (action.requestId !== state.requestId) {
        return state;
      }
      const error = classifyDashboardError(action.error);
      if (error === 'aborted') {
        return state;
      }
      return { ...state, status: 'error', error };
    }
    default:
      return state;
  }
}

export interface DashboardRequest<T> {
  state: DashboardRequestState<T>;
  // Starts a new attempt, aborting the current one. Callers offer it only
  // for retryable errors.
  reload: () => void;
}

/**
 * Runs load(signal) on mount and on every reload(). load must be stable
 * (defined outside the component), or every render would start a request.
 */
export function useDashboardRequest<T>(load: (signal: AbortSignal) => Promise<T>): DashboardRequest<T> {
  const [state, dispatch] = useReducer(
    dashboardRequestReducer<T>,
    undefined,
    initialDashboardRequestState<T>
  );
  const [reloadCount, setReloadCount] = useState(0);
  const lastRequestId = useRef(0);

  useEffect(() => {
    const controller = new AbortController();
    const requestId = ++lastRequestId.current;
    dispatch({ type: 'start', requestId });

    load(controller.signal).then(
      (data) => {
        if (!controller.signal.aborted) {
          dispatch({ type: 'success', requestId, data });
        }
      },
      (error: unknown) => {
        if (!controller.signal.aborted) {
          dispatch({ type: 'failure', requestId, error });
        }
      }
    );

    return () => controller.abort();
  }, [load, reloadCount]);

  const reload = useCallback(() => setReloadCount((count) => count + 1), []);

  return { state, reload };
}
