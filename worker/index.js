import { Buffer } from 'node:buffer';
import { AppError, fail, randomToken, seal, unseal, frontendURL, assertOrigin, readJSON, cookie, oauthCookie } from './lib/security.mjs';
import { authorFor, validDate } from './lib/plan.mjs';
import { GitHub } from './lib/github.mjs';
import { preparePublication, publish } from './lib/publish.mjs';

function configured(env) { return !!(env.GITHUB_CLIENT_ID && env.GITHUB_CLIENT_SECRET && env.SESSION_KEY && env.GITHUB_APP_SLUG); }
function apiOrigin(request, env) { return env.API_ORIGIN || new URL(request.url).origin; }
function json(value, status = 200) { return new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json; charset=utf-8' } }); }

async function oauth(request, env, fetcher) {
  if (!configured(env)) fail('not_configured', 'Вход подключается после регистрации GitHub App. Превью уже работает без входа.', 500);
  const url = new URL(request.url), origin = apiOrigin(request, env);
  if (url.pathname === '/oauth/start') {
    const nonce = url.searchParams.get('nonce');
    if (!/^[a-f\d]{64}$/.test(nonce || '')) fail('oauth_nonce', 'Начните вход кнопкой на странице конструктора.');
    const state = randomToken(), verifier = randomToken();
    const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
    const target = new URL('https://github.com/login/oauth/authorize');
    target.search = new URLSearchParams({ client_id: env.GITHUB_CLIENT_ID, redirect_uri: origin + '/oauth/callback', state, code_challenge: Buffer.from(digest).toString('base64url'), code_challenge_method: 'S256', prompt: 'select_account' });
    const encrypted = await seal({ state, verifier, nonce }, env.SESSION_KEY, 'oauth', 600);
    return new Response(null, { status: 302, headers: { Location: target.href, 'Set-Cookie': oauthCookie(encrypted, origin.startsWith('https:')) } });
  }
  if (url.pathname !== '/oauth/callback') fail('not_found', 'Страница не найдена.', 404);
  const flow = await unseal(cookie(request, 'af_oauth'), env.SESSION_KEY, 'oauth');
  if (url.searchParams.get('state') !== flow.state || !url.searchParams.get('code') || url.searchParams.has('error')) fail('oauth_state', 'Вход отменён или проверка безопасности не прошла. Начните вход заново.');
  const response = await fetcher('https://github.com/login/oauth/access_token', { method: 'POST', signal: AbortSignal.timeout(20000), redirect: 'manual', headers: { Accept: 'application/json', 'Content-Type': 'application/json', 'User-Agent': 'Activity-Forge' }, body: JSON.stringify({ client_id: env.GITHUB_CLIENT_ID, client_secret: env.GITHUB_CLIENT_SECRET, code: url.searchParams.get('code'), redirect_uri: origin + '/oauth/callback', code_verifier: flow.verifier }) });
  if (!response.ok) { await response.body?.cancel(); fail('oauth_exchange', 'GitHub не подтвердил вход. Попробуйте заново.'); }
  const data = await response.json();
  if (data.error || typeof data.access_token !== 'string' || data.access_token.length > 4096) fail('oauth_exchange', 'GitHub не выдал доступ. Попробуйте заново.');
  const user = await new GitHub(data.access_token, fetcher).request('/user'); authorFor(user);
  const session = await seal({ token: data.access_token, userId: user.id, login: user.login }, env.SESSION_KEY, 'auth', Math.min(3600, Number(data.expires_in) || 3600));
  const target = frontendURL(env); target.hash = new URLSearchParams({ session, nonce: flow.nonce }).toString();
  return new Response(null, { status: 302, headers: { Location: target.href, 'Set-Cookie': oauthCookie('', origin.startsWith('https:'), 0) } });
}

async function api(request, env, fetcher) {
  const url = new URL(request.url);
  if (url.pathname === '/api/config' && request.method === 'GET') return json({ configured: configured(env), frontend: frontendURL(env).href, appUrl: env.GITHUB_APP_SLUG ? `https://github.com/apps/${env.GITHUB_APP_SLUG}` : null, privacyVersion: 1 });
  assertOrigin(request, env);
  if (request.method === 'OPTIONS') return new Response(null, { status: 204 });
  const authorization = request.headers.get('Authorization') || '';
  if (!authorization.startsWith('Bearer ')) fail('authentication_required', 'Подключите GitHub.', 401);
  const auth = await unseal(authorization.slice(7), env.SESSION_KEY, 'auth');
  const github = new GitHub(auth.token, fetcher), user = await github.request('/user');
  if (user.id !== auth.userId || user.login !== auth.login) fail('account_changed', 'Аккаунт изменился. Подключите GitHub ещё раз.', 401);
  authorFor(user);
  if (url.pathname === '/api/me' && request.method === 'GET') {
    const { installation } = await github.account(env.GITHUB_APP_SLUG);
    return json({ id: user.id, login: user.login, author: authorFor(user), installation: installation ? { installed: true, selection: installation.repository_selection, permissions: installation.permissions } : { installed: false }, installUrl: `https://github.com/apps/${env.GITHUB_APP_SLUG}/installations/new`, revokeUrl: 'https://github.com/settings/apps/authorizations', uninstallUrl: 'https://github.com/settings/installations' });
  }
  if (url.pathname === '/api/calendar' && request.method === 'GET') {
    const start = url.searchParams.get('start'), end = url.searchParams.get('end');
    if (!validDate(start) || !validDate(end) || start > end || start < '1970-01-01' || (Date.parse(end) - Date.parse(start)) / 86400000 > 365 || end > new Date().toISOString().slice(0, 10)) fail('invalid_period', 'Выберите прошедший период до 366 дней.');
    return json(await github.calendar(start, end, user.login));
  }
  if (url.pathname === '/api/revoke' && request.method === 'POST') {
    const response = await fetcher(`https://api.github.com/applications/${encodeURIComponent(env.GITHUB_CLIENT_ID)}/grant`, { method: 'DELETE', signal: AbortSignal.timeout(20000), redirect: 'manual', headers: { Accept: 'application/vnd.github+json', 'Content-Type': 'application/json', 'User-Agent': 'Activity-Forge', Authorization: `Basic ${Buffer.from(`${env.GITHUB_CLIENT_ID}:${env.GITHUB_CLIENT_SECRET}`).toString('base64')}` }, body: JSON.stringify({ access_token: auth.token }) });
    await response.body?.cancel();
    if (![204, 404].includes(response.status)) fail('revoke_failed', 'GitHub не подтвердил отзыв. Отзовите доступ в настройках GitHub.');
    return json({ revoked: true, installationRemovalUrl: 'https://github.com/settings/installations' });
  }
  if (request.method === 'POST' && ['/api/prepare', '/api/publish'].includes(url.pathname)) {
    const body = await readJSON(request), { installation } = await github.account(env.GITHUB_APP_SLUG);
    if (!installation || installation.permissions?.contents !== 'write' || installation.permissions?.repository_creation !== 'write') fail('installation_required', 'Установите приложение в личный аккаунт с разрешениями Contents и Repository creation.', 403);
    return json(await (url.pathname === '/api/prepare' ? preparePublication(body, github, env, user) : publish(body, github, env, user)));
  }
  fail('not_found', 'Маршрут не найден.', 404);
}

export async function handleRequest(request, env, fetcher = globalThis.fetch.bind(globalThis)) {
  let response;
  try {
    const url = new URL(request.url);
    if (url.pathname.startsWith('/oauth/')) {
      if (request.method !== 'GET') fail('invalid_method', 'Метод запрещён.');
      response = await oauth(request, env, fetcher);
    } else if (url.pathname.startsWith('/api/')) response = await api(request, env, fetcher);
    else response = json({ service: 'Activity Forge API', frontend: frontendURL(env).href, status: configured(env) ? 'ready' : 'awaiting_github_app', persistence: 'none' });
  } catch (error) {
    // No request, token, URL, payload, exception or access logging.
    response = error instanceof AppError ? json({ code: error.code, message: error.message }, error.status) : json({ code: 'temporarily_unavailable', message: 'Сервис временно недоступен. Сохраните план и повторите запрос.' }, 500);
  }
  const headers = new Headers(response.headers), origin = request.headers.get('Origin');
  headers.set('Cache-Control', 'no-store'); headers.set('Referrer-Policy', 'no-referrer'); headers.set('X-Content-Type-Options', 'nosniff'); headers.set('Content-Security-Policy', "default-src 'none'; frame-ancestors 'none'"); headers.set('Vary', 'Origin');
  if (origin === frontendURL(env).origin) {
    headers.set('Access-Control-Allow-Origin', origin); headers.set('Access-Control-Allow-Methods', 'GET, POST, OPTIONS'); headers.set('Access-Control-Allow-Headers', 'Authorization, Content-Type'); headers.set('Access-Control-Max-Age', '600');
  }
  return new Response(response.body, { status: response.status, headers });
}
export default { fetch: (request, env) => handleRequest(request, env) };
