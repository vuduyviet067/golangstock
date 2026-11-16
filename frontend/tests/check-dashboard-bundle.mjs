// Checks a production build (run `vite build` first) for the dashboard code
// splitting: DashboardView must be its own chunk, loaded on demand, and the
// entry chunk must not contain the dashboard API code, so a user's browser
// never downloads it.
//
// Usage, from frontend/:  node tests/check-dashboard-bundle.mjs

import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const distDir = join(dirname(dirname(fileURLToPath(import.meta.url))), 'dist');
const assetsDir = join(distDir, 'assets');

const failures = [];
const check = (ok, message) => {
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${message}`);
  if (!ok) failures.push(message);
};

const html = readFileSync(join(distDir, 'index.html'), 'utf8');
const entryName = html.match(/<script type="module"[^>]*src="\/assets\/([^"]+\.js)"/)?.[1];
check(Boolean(entryName), `index.html loads an entry chunk (${entryName ?? 'none'})`);

const chunks = readdirSync(assetsDir).filter((name) => name.endsWith('.js'));
const dashboardChunks = chunks.filter((name) => name.startsWith('DashboardView-'));
check(dashboardChunks.length === 1, `one DashboardView chunk (${dashboardChunks.join(', ') || 'none'})`);

if (entryName && dashboardChunks.length === 1) {
  const entry = readFileSync(join(assetsDir, entryName), 'utf8');
  const dashboard = readFileSync(join(assetsDir, dashboardChunks[0]), 'utf8');

  check(!html.includes(dashboardChunks[0]), 'index.html does not preload the DashboardView chunk');
  check(entry.includes(dashboardChunks[0]), 'the entry chunk imports DashboardView on demand');
  // recharts-wrapper is a class name in Recharts' own code; status-pie,
  // stock-chart, range-filter and stock-movement are ours.
  const ownMarkers = ['summary-card', 'status-pie', 'stock-chart', 'range-filter', 'stock-movement'];
  for (const marker of ['/api/v1/dashboard', '/summary', '/stock-movements', 'recharts-wrapper', ...ownMarkers]) {
    check(!entry.includes(marker), `the entry chunk does not contain "${marker}"`);
  }
  check(dashboard.includes('/api/v1/dashboard'), 'the DashboardView chunk contains the dashboard API code');
  check(dashboard.includes('summary-card'), 'the DashboardView chunk contains the summary cards');
  check(dashboard.includes('recharts-wrapper'), 'the DashboardView chunk contains Recharts');
  check(dashboard.includes('status-pie'), 'the DashboardView chunk contains the status pie');
  check(dashboard.includes('stock-chart'), 'the DashboardView chunk contains the stock movement bar chart');
  check(dashboard.includes('range-filter'), 'the DashboardView chunk contains the date range filter');
  check(dashboard.includes('stock-movement'), 'the DashboardView chunk contains the stock movement section');
  check(dashboard.includes('/stock-movements'), 'the DashboardView chunk contains the stock movement API call');
}

if (failures.length > 0) {
  console.error(`\nDashboard bundle check FAILED (${failures.length})`);
  process.exit(1);
}
console.log('\nDashboard bundle check passed');
