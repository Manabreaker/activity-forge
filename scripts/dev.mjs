import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import { handleRequest } from '../worker/index.js';
const root = resolve('dist/pages');
const env = { ...process.env, FRONTEND_URL: 'http://127.0.0.1:4317/', API_ORIGIN: 'http://127.0.0.1:4318' };
createServer(async (req, res) => {
  try {
    const url = new URL(req.url, 'http://127.0.0.1:4317');
    const pathname = decodeURIComponent(url.pathname);
    if (pathname.endsWith('/config.js')) { res.setHeader('Content-Type', 'text/javascript'); res.end("export const API_ORIGIN = 'http://127.0.0.1:4318';"); return; }
    const path = resolve(root, '.' + (pathname === '/' ? '/index.html' : pathname));
    if (!path.startsWith(root + '/')) { res.writeHead(404); res.end(); return; }
    let data = await readFile(path);
    if (path.endsWith('index.html')) data = data.toString().replace('connect-src \'self\' https://activity-forge-calendar.manabreaker.chatgpt.site', 'connect-src \'self\' http://127.0.0.1:4318');
    const type = { '.html': 'text/html; charset=utf-8', '.css': 'text/css', '.js': 'text/javascript', '.wasm': 'application/wasm', '.svg': 'image/svg+xml' }[extname(path)] || 'application/octet-stream';
    res.setHeader('Content-Type', type); res.end(data);
  } catch { res.writeHead(404); res.end('Not found'); }
}).listen(4317, '127.0.0.1', () => console.log('Frontend: http://127.0.0.1:4317'));
createServer(async (req, res) => {
  const options = { method: req.method, headers: req.headers };
  if (!['GET', 'HEAD'].includes(req.method)) { options.body = req; options.duplex = 'half'; }
  try {
    const response = await handleRequest(new Request('http://127.0.0.1:4318' + req.url, options), env);
    res.writeHead(response.status, Object.fromEntries(response.headers)); res.end(Buffer.from(await response.arrayBuffer()));
  } catch { res.writeHead(500); res.end(); }
}).listen(4318, '127.0.0.1', () => console.log('API: http://127.0.0.1:4318 (no credentials loaded unless provided as environment variables)'));
