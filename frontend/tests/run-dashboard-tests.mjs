// Runs the dashboard tests without adding dependencies:
//   1. type-checks tests/ (and the sources they import) with the project's tsc;
//   2. bundles each tests/*.test.ts with the esbuild that ships with Vite,
//      into a temporary directory outside the repository;
//   3. runs the bundles with node:test once per time zone in TIME_ZONES, so
//      the results cannot depend on the time zone of the machine.
//
// Usage, from frontend/:  node tests/run-dashboard-tests.mjs
// Needs no backend, database or network.

import esbuild from 'esbuild';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readdirSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const TIME_ZONES = ['UTC', 'America/Santiago', 'Pacific/Kiritimati'];

const testsDir = dirname(fileURLToPath(import.meta.url));
const frontendDir = dirname(testsDir);
const tscPath = join(frontendDir, 'node_modules', 'typescript', 'bin', 'tsc');

function runNode(args, env = process.env) {
  const result = spawnSync(process.execPath, args, { stdio: 'inherit', env });
  if (result.error) {
    throw result.error;
  }
  return result.status ?? 1;
}

console.log('> tsc --noEmit -p tests/tsconfig.json');
if (runNode([tscPath, '--noEmit', '-p', join(testsDir, 'tsconfig.json')]) !== 0) {
  console.error('\nDashboard tests FAILED: type errors');
  process.exit(1);
}

const testFiles = readdirSync(testsDir)
  .filter((name) => name.endsWith('.test.ts'))
  .sort()
  .map((name) => join(testsDir, name));
if (testFiles.length === 0) {
  console.error('No tests/*.test.ts files found');
  process.exit(1);
}

const outDir = mkdtempSync(join(tmpdir(), 'gostock-dashboard-tests-'));
const failedTimeZones = [];
try {
  await esbuild.build({
    entryPoints: testFiles,
    outdir: outDir,
    bundle: true,
    platform: 'node',
    format: 'esm',
    target: 'node22',
    outExtension: { '.js': '.mjs' },
    // CommonJS dependencies bundled into ESM (react-dom/server) still call
    // require() for Node built-ins such as "stream".
    banner: {
      js: "import { createRequire as __createRequire } from 'node:module'; const require = __createRequire(import.meta.url);",
    },
    sourcemap: 'inline',
    logLevel: 'warning',
  });
  const bundles = testFiles.map((file) => join(outDir, `${basename(file, '.ts')}.mjs`));

  for (const timeZone of TIME_ZONES) {
    console.log(`\n> node --test (TZ=${timeZone})`);
    const status = runNode(['--test', '--test-reporter=spec', ...bundles], {
      ...process.env,
      TZ: timeZone,
      DASHBOARD_TEST_TZ: timeZone,
    });
    if (status !== 0) {
      failedTimeZones.push(timeZone);
    }
  }
} finally {
  rmSync(outDir, { recursive: true, force: true });
}

if (failedTimeZones.length > 0) {
  console.error(`\nDashboard tests FAILED in: ${failedTimeZones.join(', ')}`);
  process.exit(1);
}
console.log(`\nDashboard tests passed in: ${TIME_ZONES.join(', ')}`);
