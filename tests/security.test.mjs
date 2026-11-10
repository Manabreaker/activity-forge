import test from 'node:test';
import assert from 'node:assert/strict';
import { seal, unseal } from '../worker/lib/security.mjs';

test('encrypted browser envelope hides credentials and binds purpose and expiry', async () => {
  const key = Buffer.alloc(32, 17).toString('base64');
  const token = await seal({ token: 'sensitive-test-value' }, key, 'auth', 60);
  assert.ok(!token.includes('sensitive-test-value'));
  assert.equal((await unseal(token, key, 'auth')).token, 'sensitive-test-value');
  await assert.rejects(unseal(token, key, 'oauth'));
  await assert.rejects(unseal(token.slice(0, -4) + 'zzzz', key, 'auth'));
  const expired = await seal({ token: 'expired' }, key, 'auth', -1);
  await assert.rejects(unseal(expired, key, 'auth'));
});
