// Publish locally verified output without requiring a hosted Actions runner.
import { execFileSync } from 'node:child_process';
import { cp, mkdtemp, rm, readFile, readdir } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { tmpdir } from 'node:os';
const repository = process.argv[2];
if (!/^[a-z\d-]+\/[a-z\d._-]+$/i.test(repository || '')) throw new Error('usage: node scripts/publish-pages.mjs OWNER/REPO');
const root = resolve('dist/pages');
await readFile(join(root, 'index.html')); await readFile(join(root, 'generator.wasm'));
const user = JSON.parse(execFileSync('gh', ['api', 'user', '--jq', '{login,id}'], { encoding: 'utf8' }));
if (repository.split('/')[0].toLowerCase() !== user.login.toLowerCase()) throw new Error('publish only to your own account');
const dir = await mkdtemp(join(tmpdir(), 'activity-forge-pages-'));
const git = args => execFileSync('git', ['-C', dir, ...args], { encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] });
try {
  git(['init', '-b', 'gh-pages']); git(['remote', 'add', 'origin', `https://github.com/${repository}.git`]);
  const refs = git(['ls-remote', '--heads', 'origin', 'gh-pages']).trim();
  if (refs) { git(['fetch', '--depth=1', 'origin', 'gh-pages']); git(['checkout', '-B', 'gh-pages', 'FETCH_HEAD']); git(['rm', '-r', '--ignore-unmatch', '.']); }
  for (const item of await readdir(root)) await cp(join(root, item), join(dir, item), { recursive: true });
  git(['config', 'user.name', user.login]); git(['config', 'user.email', `${user.id}+${user.login}@users.noreply.github.com`]);
  git(['add', '.']); git(['diff', '--cached', '--check']);
  if (!git(['status', '--porcelain']).trim()) { console.log('Pages output already matches gh-pages.'); }
  else { git(['commit', '-m', 'Publish verified Activity Forge frontend']); git(['push', '-u', 'origin', 'gh-pages']); console.log(`Published gh-pages to ${repository}; GitHub Pages will build it.`); }
} finally { await rm(dir, { recursive: true, force: true }); }
