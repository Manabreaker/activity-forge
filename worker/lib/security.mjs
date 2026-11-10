import { Buffer } from 'node:buffer';

export class AppError extends Error {
  constructor(code, message, status = 400) { super(message); this.code = code; this.status = status; }
}

export function fail(code, message, status = 400) { throw new AppError(code, message, status); }
export function randomToken() { return Buffer.from(crypto.getRandomValues(new Uint8Array(32))).toString('base64url'); }

async function envelopeKey(secret) {
  const bytes = Buffer.from(secret || '', 'base64');
  if (bytes.length !== 32) fail('not_configured', 'Сервис авторизации ещё не настроен.', 500);
  return crypto.subtle.importKey('raw', bytes, 'AES-GCM', false, ['encrypt', 'decrypt']);
}

export async function seal(value, secret, purpose, seconds) {
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const body = new TextEncoder().encode(JSON.stringify({ ...value, exp: Math.floor(Date.now() / 1000) + seconds }));
  const encrypted = await crypto.subtle.encrypt({ name: 'AES-GCM', iv, additionalData: new TextEncoder().encode(purpose) }, await envelopeKey(secret), body);
  return Buffer.concat([iv, new Uint8Array(encrypted)]).toString('base64url');
}

export async function unseal(token, secret, purpose) {
  try {
    if (typeof token !== 'string' || token.length > 16000 || !/^[\w-]+$/.test(token)) throw new Error();
    const bytes = Buffer.from(token, 'base64url');
    const data = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: bytes.subarray(0, 12), additionalData: new TextEncoder().encode(purpose) }, await envelopeKey(secret), bytes.subarray(12));
    const value = JSON.parse(new TextDecoder().decode(data));
    if (!Number.isInteger(value.exp) || value.exp <= Date.now() / 1000) throw new Error();
    return value;
  } catch { fail('session_expired', 'Сессия истекла. Подключите GitHub ещё раз.', 401); }
}

export async function sha256(value) {
  return Buffer.from(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))).toString('hex');
}

export function frontendURL(env) {
  const url = new URL(env.FRONTEND_URL || 'https://manabreaker.github.io/activity-forge/');
  if (url.protocol !== 'https:' && url.hostname !== '127.0.0.1' && url.hostname !== 'localhost') fail('invalid_config', 'Нужен HTTPS адрес интерфейса.', 500);
  url.hash = ''; url.search = '';
  return url;
}

export function assertOrigin(request, env) {
  if (request.headers.get('Origin') !== frontendURL(env).origin) fail('origin_denied', 'Запрос с этого сайта запрещён.', 403);
}

export async function readJSON(request, limit = 2 << 20) {
  if (!request.headers.get('Content-Type')?.startsWith('application/json')) fail('invalid_content_type', 'Нужен JSON запрос.');
  if (Number(request.headers.get('Content-Length')) > limit) fail('request_too_large', 'Слишком большой запрос.');
  const reader = request.body?.getReader();
  if (!reader) fail('invalid_json', 'Пустой запрос.');
  const chunks = []; let length = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    length += value.byteLength;
    if (length > limit) { await reader.cancel(); fail('request_too_large', 'Слишком большой запрос.'); }
    chunks.push(value);
  }
  try { return JSON.parse(Buffer.concat(chunks).toString('utf8')); }
  catch { fail('invalid_json', 'Некорректный JSON.'); }
}

export function cookie(request, name) {
  return (request.headers.get('Cookie') || '').split(';').map(x => x.trim()).find(x => x.startsWith(name + '='))?.slice(name.length + 1) || '';
}

export function oauthCookie(value, secure = true, maxAge = 600) {
  return `af_oauth=${value}; Path=/oauth/; Max-Age=${maxAge}; HttpOnly; SameSite=Lax${secure ? '; Secure' : ''}`;
}
