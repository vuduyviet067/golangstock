// Summarizes a `go test -json` report and enforces the CI rules on it.
//
// Usage: node go-test-report.mjs REPORT.json [--no-skip] [--require=TestA,TestB]
//
// A package line "ok" is not evidence that its tests ran: a test whose
// subtests all skip still reports PASS. This script reads every test and
// subtest result instead, and exits 1 when:
//   - any test, subtest or package failed, or a package failed to build;
//   - a test started but never reported a result (panic, timeout);
//   - --no-skip is given and any test or subtest was skipped;
//   - a --require'd test did not run, did not pass, or has a subtest that
//     did not pass;
//   - the report holds no test at all, or a line that is not test2json.
// Needs only Node.js; no dependencies.

import { appendFileSync, readFileSync } from 'node:fs';

const MAX_OUTPUT_LINES = 40;

function usage(message) {
  console.error(`${message}\nusage: node go-test-report.mjs REPORT.json [--no-skip] [--require=TestA,TestB]`);
  process.exit(2);
}

let reportPath = null;
let noSkip = false;
const required = [];
for (const arg of process.argv.slice(2)) {
  if (arg === '--no-skip') {
    noSkip = true;
  } else if (arg.startsWith('--require=')) {
    required.push(...arg.slice('--require='.length).split(',').map((s) => s.trim()).filter(Boolean));
  } else if (reportPath === null && !arg.startsWith('--')) {
    reportPath = arg;
  } else {
    usage(`unexpected argument: ${arg}`);
  }
}
if (reportPath === null) {
  usage('missing report path');
}

const key = (pkg, test) => `${pkg}\u0000${test}`;
const tests = new Map(); // key -> { pkg, test, result: 'pass'|'fail'|'skip'|null, output: [] }
const packages = new Map(); // pkg -> { result, output: [] }
const buildFailures = new Set();
const buildOutput = new Map(); // import path -> lines
const strayLines = [];

for (const line of readFileSync(reportPath, 'utf8').split(/\r?\n/)) {
  if (line.trim() === '') {
    continue;
  }
  let ev;
  try {
    ev = JSON.parse(line);
  } catch {
    strayLines.push(line);
    continue;
  }
  if (ev.Action === 'build-output') {
    const lines = buildOutput.get(ev.ImportPath) ?? [];
    lines.push(ev.Output ?? '');
    buildOutput.set(ev.ImportPath, lines);
    continue;
  }
  if (ev.Action === 'build-fail') {
    buildFailures.add(ev.ImportPath);
    continue;
  }
  if (!ev.Package) {
    continue;
  }
  if (ev.Test) {
    const k = key(ev.Package, ev.Test);
    const entry = tests.get(k) ?? { pkg: ev.Package, test: ev.Test, result: null, output: [] };
    if (ev.Action === 'output') {
      entry.output.push(ev.Output ?? '');
    } else if (ev.Action === 'pass' || ev.Action === 'fail' || ev.Action === 'skip') {
      entry.result = ev.Action;
    }
    tests.set(k, entry);
  } else {
    const entry = packages.get(ev.Package) ?? { result: null, output: [] };
    if (ev.Action === 'output') {
      entry.output.push(ev.Output ?? '');
    } else if (ev.Action === 'pass' || ev.Action === 'fail' || ev.Action === 'skip') {
      entry.result = ev.Action;
    }
    packages.set(ev.Package, entry);
  }
}

const all = [...tests.values()];
const isSub = (t) => t.test.includes('/');
const count = (list, result) => list.filter((t) => t.result === result).length;
const top = all.filter((t) => !isSub(t));
const subs = all.filter(isSub);
const name = (t) => `${t.pkg} ${t.test}`;
const tail = (lines) => lines.join('').trimEnd().split('\n').slice(-MAX_OUTPUT_LINES).join('\n');

const problems = [];

console.log(`go test report: ${reportPath}`);
console.log(`  top-level tests: ${top.length} (pass ${count(top, 'pass')}, fail ${count(top, 'fail')}, skip ${count(top, 'skip')})`);
console.log(`  subtests:        ${subs.length} (pass ${count(subs, 'pass')}, fail ${count(subs, 'fail')}, skip ${count(subs, 'skip')})`);
console.log(`  packages:        ${packages.size} (fail ${[...packages.values()].filter((p) => p.result === 'fail').length})`);

if (all.length === 0) {
  problems.push('the report holds no test results');
}
for (const line of strayLines) {
  problems.push(`line is not test2json output: ${line.slice(0, 200)}`);
}
for (const path of buildFailures) {
  problems.push(`build failed: ${path}\n${tail(buildOutput.get(path) ?? [])}`);
}
for (const t of all.filter((t) => t.result === 'fail')) {
  problems.push(`FAIL ${name(t)}\n${tail(t.output)}`);
}
for (const t of all.filter((t) => t.result === null)) {
  problems.push(`no result (panic or timeout?): ${name(t)}\n${tail(t.output)}`);
}
for (const [pkg, p] of packages) {
  const failedTests = all.some((t) => t.pkg === pkg && t.result === 'fail');
  if (p.result === 'fail' && !failedTests) {
    problems.push(`package failed outside any test: ${pkg}\n${tail(p.output)}`);
  }
}

const skipped = all.filter((t) => t.result === 'skip');
if (skipped.length > 0) {
  console.log(`\nSkipped (${skipped.length}):`);
  for (const t of skipped) {
    const reason = t.output.map((l) => l.trim()).filter((l) => l && !l.startsWith('=== ') && !l.startsWith('--- ')).pop() ?? '';
    console.log(`  ${name(t)}${reason ? `: ${reason}` : ''}`);
  }
  if (noSkip) {
    problems.push(`${skipped.length} test(s) skipped; --no-skip forbids skips (see the list above)`);
  }
}

if (required.length > 0) {
  console.log('\nRequired tests:');
  for (const testName of required) {
    const runs = top.filter((t) => t.test === testName);
    if (runs.length === 0) {
      problems.push(`required test did not run: ${testName}`);
      console.log(`  ${testName}: NOT RUN`);
      continue;
    }
    for (const run of runs) {
      const children = subs.filter((t) => t.pkg === run.pkg && t.test.startsWith(`${testName}/`));
      const bad = children.filter((t) => t.result !== 'pass');
      console.log(`  ${testName} (${run.pkg}): ${String(run.result).toUpperCase()}, subtests ${children.length - bad.length}/${children.length} passed`);
      if (run.result !== 'pass') {
        problems.push(`required test did not pass: ${name(run)} (${run.result})`);
      }
      for (const child of bad) {
        problems.push(`required test has a subtest that did not pass: ${name(child)} (${child.result})`);
      }
    }
  }
}

if (process.env.GITHUB_STEP_SUMMARY) {
  const lines = [
    `### go test: ${reportPath}`,
    '',
    '| | total | pass | fail | skip |',
    '|---|---|---|---|---|',
    `| top-level tests | ${top.length} | ${count(top, 'pass')} | ${count(top, 'fail')} | ${count(top, 'skip')} |`,
    `| subtests | ${subs.length} | ${count(subs, 'pass')} | ${count(subs, 'fail')} | ${count(subs, 'skip')} |`,
    '',
    problems.length === 0 ? 'Result: OK' : `Result: ${problems.length} problem(s)`,
    '',
  ];
  appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${lines.join('\n')}\n`);
}

if (problems.length > 0) {
  console.error(`\n${problems.length} problem(s):`);
  for (const p of problems) {
    console.error(`\n- ${p}`);
  }
  process.exit(1);
}
console.log('\nOK');
