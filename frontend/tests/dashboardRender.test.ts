import { afterEach, beforeEach, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { createElement, type ComponentType } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { MainTabs } from '../src/components/MainTabs';
import { DashboardPanel } from '../src/components/DashboardPanel';
import DashboardView, { DashboardErrorNotice } from '../src/components/DashboardView';
import { visibleTabs } from '../src/utils/dashboardTabs';
import type { DashboardErrorKind } from '../src/utils/dashboardErrors';

// Server-side rendering needs no DOM. It renders the first pass only: no
// effects run, so no request starts.

const noop = () => {};
// The components are plain .jsx, so their props are untyped.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const render = (type: ComponentType<any>, props: Record<string, unknown> = {}) =>
  renderToStaticMarkup(createElement(type, props));
const count = (html: string, pattern: RegExp) => (html.match(pattern) ?? []).length;

const globalWithFetch = globalThis as { fetch: typeof fetch };
const originalFetch = globalWithFetch.fetch;
let fetchCalls = 0;
beforeEach(() => {
  fetchCalls = 0;
  globalWithFetch.fetch = async () => {
    fetchCalls += 1;
    throw new Error('no request is expected while rendering');
  };
});
afterEach(() => {
  globalWithFetch.fetch = originalFetch;
});

describe('MainTabs', () => {
  it('shows an admin both tabs, Dashboard selected', () => {
    const html = render(MainTabs, { tabs: visibleTabs({ id: 1, role: 'admin' }), activeTab: 'dashboard', onChange: noop });
    assert.equal(count(html, /role="tab"/g), 2);
    assert.match(html, /role="tablist"/);
    assert.match(html, /id="main-tab-dashboard"[^>]*aria-selected="true"[^>]*aria-controls="main-panel-dashboard"[^>]*tabindex="0"[^>]*>Dashboard</);
    assert.match(html, /id="main-tab-products"[^>]*aria-selected="false"[^>]*tabindex="-1"[^>]*>Sản phẩm</);
  });

  it('marks the selected tab', () => {
    const html = render(MainTabs, { tabs: ['dashboard', 'products'], activeTab: 'products', onChange: noop });
    assert.match(html, /id="main-tab-products"[^>]*aria-selected="true"/);
    assert.match(html, /id="main-tab-dashboard"[^>]*aria-selected="false"/);
  });

  it('shows a user only Sản phẩm, with no trace of the Dashboard', () => {
    const html = render(MainTabs, { tabs: visibleTabs({ id: 2, role: 'user' }), activeTab: 'products', onChange: noop });
    assert.equal(count(html, /role="tab"/g), 1);
    assert.match(html, /aria-selected="true"[^>]*>Sản phẩm</);
    assert.equal(/dashboard/i.test(html), false, html);
  });
});

// The summary cards are tested in dashboardSummaryCards.test.ts.

describe('DashboardErrorNotice', () => {
  const notice = (kind: DashboardErrorKind, handlers = { onRetry: noop, onReauthenticate: noop }) =>
    render(DashboardErrorNotice, { kind, ...handlers });
  const buttons = (html: string) => [...html.matchAll(/<button[^>]*>([^<]*)<\/button>/g)].map((m) => m[1]);

  it('offers Thử lại for network failures and 5xx', () => {
    assert.deepEqual(buttons(notice('network')), ['Thử lại']);
    assert.deepEqual(buttons(notice('server')), ['Thử lại']);
    assert.match(notice('network'), /Không thể kết nối tới máy chủ/);
  });

  it('offers a session check, not a retry, for 401 and 403', () => {
    assert.deepEqual(buttons(notice('unauthenticated')), ['Đăng nhập lại']);
    assert.deepEqual(buttons(notice('forbidden')), ['Cập nhật quyền']);
    assert.match(notice('forbidden'), /Không có quyền xem Dashboard/);
  });

  it('offers nothing for 400 and unexpected errors', () => {
    assert.deepEqual(buttons(notice('invalid')), []);
    assert.deepEqual(buttons(notice('unexpected')), []);
  });

  it('shows no button without a handler', () => {
    assert.deepEqual(buttons(render(DashboardErrorNotice, { kind: 'network' })), []);
  });

  it('is announced as an alert', () => {
    assert.match(notice('server'), /role="alert"/);
  });

  // The markup shows the label only; this checks which handler the button
  // calls. DashboardErrorNotice uses no hooks, so it can be called directly.
  it('wires the button to the right handler', () => {
    type Node = { type?: unknown; props?: { children?: unknown; onClick?: unknown } };
    const findButtons = (node: unknown): Node[] => {
      if (Array.isArray(node)) return node.flatMap(findButtons);
      if (!node || typeof node !== 'object') return [];
      const element = node as Node;
      const own = element.type === 'button' ? [element] : [];
      return [...own, ...findButtons(element.props?.children)];
    };
    const onRetry = () => {};
    const onReauthenticate = () => {};
    const handlersFor = (kind: DashboardErrorKind) =>
      findButtons((DashboardErrorNotice as (props: object) => unknown)({ kind, onRetry, onReauthenticate })).map(
        (button) => button.props?.onClick
      );

    assert.deepEqual(handlersFor('network'), [onRetry]);
    assert.deepEqual(handlersFor('server'), [onRetry]);
    assert.deepEqual(handlersFor('unauthenticated'), [onReauthenticate]);
    assert.deepEqual(handlersFor('forbidden'), [onReauthenticate]);
    assert.deepEqual(handlersFor('invalid'), []);
    assert.deepEqual(handlersFor('unexpected'), []);
  });
});

describe('DashboardView and DashboardPanel', () => {
  it('DashboardView starts in the loading state without requesting anything', () => {
    const html = render(DashboardView, { onReauthenticate: noop });
    assert.match(html, /Tổng quan tồn kho/);
    assert.match(html, /role="status"[^>]*>.*Đang tải số liệu/);
    assert.equal(fetchCalls, 0);
  });

  it('DashboardPanel shows the loading fallback while the code loads', () => {
    const html = render(DashboardPanel, { onReauthenticate: noop });
    assert.match(html, /role="status"[^>]*>.*Đang tải Dashboard/);
    assert.equal(fetchCalls, 0);
  });
});
