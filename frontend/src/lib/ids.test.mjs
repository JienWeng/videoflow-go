import test from 'node:test';
import assert from 'node:assert/strict';
import { randomId } from './ids.ts';

test('randomId works when crypto.randomUUID is not available', () => {
  const original = crypto.randomUUID;
  Object.defineProperty(crypto, 'randomUUID', { value: undefined, configurable: true });
  try {
    const a = randomId();
    const b = randomId();
    assert.match(a, /^[0-9a-f]{32}$/);
    assert.notEqual(a, b);
  } finally {
    Object.defineProperty(crypto, 'randomUUID', { value: original, configurable: true });
  }
});
