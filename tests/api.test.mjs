import test from 'node:test';
import assert from 'node:assert/strict';
import { handleRequest } from '../worker/index.js';
import { GitHub } from '../worker/lib/github.mjs';
const env = { FRONTEND_URL: 'https://example.github.io/activity-forge/', API_ORIGIN: 'https://api.example.test', SESSION_KEY: Buffer.alloc(32, 9).toString('base64'), GITHUB_CLIENT_ID: 'test-client-id', GITHUB_CLIENT_SECRET: 'test-client-secret', GITHUB_APP_SLUG: 'example-forge' };

test('CORS allows only the configured Pages origin and never enables wildcard credentials', async () => {
  const good = await handleRequest(new Request('https://api.example.test/api/config', { headers: { Origin: 'https://example.github.io' } }), env);
  assert.equal(good.status, 200); assert.equal(good.headers.get('Access-Control-Allow-Origin'), 'https://example.github.io');
  const bad = await handleRequest(new Request('https://api.example.test/api/me', { headers: { Origin: 'https://evil.test' } }), env);
  assert.equal(bad.status, 403); assert.equal(bad.headers.get('Access-Control-Allow-Origin'), null);
});

test('API fails closed without authorization and OAuth callback rejects mismatched state before exchange', async () => {
  const me = await handleRequest(new Request('https://api.example.test/api/me', { headers: { Origin: 'https://example.github.io' } }), env);
  assert.equal(me.status, 401); assert.equal(me.headers.get('Cache-Control'), 'no-store');
  const start = await handleRequest(new Request('https://api.example.test/oauth/start?nonce=' + 'a'.repeat(64)), env);
  assert.equal(start.status, 302);
  const location = new URL(start.headers.get('Location'));
  assert.equal(location.origin, 'https://github.com'); assert.equal(location.searchParams.get('code_challenge_method'), 'S256');
  const setCookie = start.headers.get('Set-Cookie'); assert.ok(setCookie.includes('HttpOnly')); assert.ok(setCookie.includes('Secure'));
  let called = false;
  const bad = await handleRequest(new Request('https://api.example.test/oauth/callback?code=example&state=wrong', { headers: { Cookie: setCookie.split(';')[0] } }), env, async () => { called = true; throw new Error(); });
  assert.equal(bad.status, 400); assert.equal(called, false);
});

test('Pages base path is preserved and an unconfigured service provides honest readiness', async () => {
  const response = await handleRequest(new Request('https://api.example.test/api/config'), { FRONTEND_URL: env.FRONTEND_URL });
  assert.equal((await response.json()).configured, false);
  const start = await handleRequest(new Request('https://api.example.test/oauth/start'), { FRONTEND_URL: env.FRONTEND_URL });
  assert.equal(start.status, 500);
});

test('OAuth handoff carries browser nonce and encrypted auth, and discards refresh token', async () => {
  const start = await handleRequest(new Request('https://api.example.test/oauth/start?nonce=' + 'b'.repeat(64)), env);
  const url = new URL(start.headers.get('Location'));
  let exchangeBody;
  const response = await handleRequest(new Request(`https://api.example.test/oauth/callback?code=example&state=${url.searchParams.get('state')}`, { headers: { Cookie: start.headers.get('Set-Cookie').split(';')[0] } }), env, async (target, init) => {
    if (target.includes('/access_token')) { exchangeBody = JSON.parse(init.body); return Response.json({ access_token: 'synthetic-provider-value', refresh_token: 'synthetic-refresh-value', expires_in: 28800 }); }
    return Response.json({ id: 12345, login: 'example-user' });
  });
  assert.equal(response.status, 302);
  const location = new URL(response.headers.get('Location'));
  assert.equal(location.origin + location.pathname, env.FRONTEND_URL);
  const hash = new URLSearchParams(location.hash.slice(1)); assert.equal(hash.get('nonce'), 'b'.repeat(64));
  assert.ok(hash.get('session')); assert.ok(!location.href.includes('synthetic-provider-value')); assert.ok(!location.href.includes('synthetic-refresh-value'));
  assert.ok(exchangeBody.code_verifier); assert.equal(exchangeBody.redirect_uri, 'https://api.example.test/oauth/callback');
});

test('OAuth and GitHub requests preserve the native Worker fetch receiver', async t => {
  const original = globalThis.fetch;
  t.after(() => { globalThis.fetch = original; });
  globalThis.fetch = function (target) {
    if (this !== globalThis) throw new TypeError('Illegal invocation');
    if (target.endsWith('/access_token')) return Promise.resolve(Response.json({ access_token: 'synthetic-runtime-value', expires_in: 3600 }));
    if (target.includes('/user/installations')) return Promise.resolve(Response.json({ installations: [] }));
    if (target.includes('/info/refs')) return Promise.resolve(new Response(new Uint8Array([1, 2, 3])));
    return Promise.resolve(Response.json({ id: 12345, login: 'example-user' }));
  };
  const start = await handleRequest(new Request('https://api.example.test/oauth/start?nonce=' + 'c'.repeat(64)), env);
  const state = new URL(start.headers.get('Location')).searchParams.get('state');
  const callback = await handleRequest(new Request(`https://api.example.test/oauth/callback?code=example&state=${state}`, { headers: { Cookie: start.headers.get('Set-Cookie').split(';')[0] } }), env);
  assert.equal(callback.status, 302);
  const session = new URLSearchParams(new URL(callback.headers.get('Location')).hash.slice(1)).get('session');
  const me = await handleRequest(new Request('https://api.example.test/api/me', { headers: { Origin: 'https://example.github.io', Authorization: 'Bearer ' + session } }), env);
  assert.equal(me.status, 200);
  assert.equal((await me.json()).login, 'example-user');
  const github = new GitHub('synthetic-runtime-value');
  assert.equal((await github.request('/user')).id, 12345);
  assert.equal((await github.gitFetch('https://github.com/example-user/calendar.git/info/refs', {})).byteLength, 3);
});
