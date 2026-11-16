// Minimal declarations of the Node built-ins the tests use. The project has
// no @types/node and the tests must not add dependencies.

declare module 'node:test' {
  interface TestContext {
    skip(message?: string): void;
  }
  type TestFn = (t: TestContext) => void | Promise<void>;
  type HookFn = () => void | Promise<void>;

  export function describe(name: string, fn: () => void): void;
  export function it(name: string, fn: TestFn): void;
  export function beforeEach(fn: HookFn): void;
  export function afterEach(fn: HookFn): void;
}

declare module 'node:assert/strict' {
  function assert(value: unknown, message?: string): asserts value;
  namespace assert {
    function ok(value: unknown, message?: string): asserts value;
    function equal<T>(actual: unknown, expected: T, message?: string): asserts actual is T;
    function notEqual(actual: unknown, expected: unknown, message?: string): void;
    function deepEqual<T>(actual: unknown, expected: T, message?: string): asserts actual is T;
    function match(value: string, pattern: RegExp, message?: string): void;
    function throws(fn: () => unknown, expected?: object, message?: string): void;
    function rejects(
      promise: Promise<unknown> | (() => Promise<unknown>),
      expected?: object,
      message?: string
    ): Promise<void>;
  }
  export default assert;
}

declare const process: {
  env: Record<string, string | undefined>;
};
