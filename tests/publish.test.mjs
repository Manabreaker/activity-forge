import test from 'node:test';
import assert from 'node:assert/strict';
import { preparePublication, publish } from '../worker/lib/publish.mjs';
import { pktLine } from '../worker/lib/git.mjs';
import { samplePlan, identity } from './fixtures.mjs';
const env = { SESSION_KEY: Buffer.alloc(32, 6).toString('base64') };

class Remote {
  constructor() { this.repo = null; this.head = null; this.creates = 0; this.pushes = 0; }
  async request(path, options = {}) {
    if (path === '/user/repos') { this.creates++; const value = JSON.parse(options.body); this.repo = { ...value, owner: { id: identity.id }, default_branch: 'main', fork: false, archived: false }; return this.repo; }
    if (path.endsWith('/git/ref/heads/main')) return { object: { sha: this.head } };
    return this.repo;
  }
  async gitFetch(url, options) {
    if (options.method === 'GET') return Buffer.concat([pktLine('# service=git-receive-pack\n'), Buffer.from('0000'), pktLine(`${this.head || '0'.repeat(40)} ${this.head ? 'refs/heads/main' : 'capabilities^{}'}\0report-status delete-refs ofs-delta\n`), Buffer.from('0000')]);
    this.pushes++; this.head = options.body.toString('utf8', 4, 100).split(' ')[1];
    return Buffer.concat([pktLine('unpack ok\n'), pktLine('ok refs/heads/main\n'), Buffer.from('0000')]);
  }
}

test('explicit frozen-plan publication is verifiable and retries never duplicate the history', async () => {
  const f = await samplePlan(), remote = new Remote();
  const prepared = await preparePublication({ ...f, repository: 'decorative-history' }, remote, env, identity);
  assert.equal(remote.creates, 0);
  const body = { ...f, ticket: prepared.ticket, confirmed: true };
  await assert.rejects(publish({ ...body, confirmed: false }, remote, env, identity));
  assert.equal(remote.creates, 0);
  const first = await publish(body, remote, env, identity);
  const second = await publish(body, remote, env, identity);
  assert.equal(first.head, second.head); assert.equal(first.commits, 3); assert.equal(remote.creates, 1); assert.equal(remote.pushes, 1);
  assert.equal(first.verified, true); assert.equal(first.private, true);
});

test('public, foreign and unmarked existing repositories are rejected before any write', async () => {
  const f = await samplePlan();
  for (const repo of [
    { owner: { id: identity.id }, private: false },
    { owner: { id: identity.id + 1 }, private: true },
    { owner: { id: identity.id }, private: true, description: 'an unrelated project' }
  ]) {
    const remote = new Remote(); remote.repo = repo;
    await assert.rejects(preparePublication({ ...f, repository: 'existing-project' }, remote, env, identity));
    assert.equal(remote.creates, 0); assert.equal(remote.pushes, 0);
  }
});

test('a different remote HEAD, even in a marked repository, stops publication', async () => {
  const f = await samplePlan(), remote = new Remote();
  const prepared = await preparePublication({ ...f, repository: 'decorative-history' }, remote, env, identity);
  const body = { ...f, ticket: prepared.ticket, confirmed: true };
  await publish(body, remote, env, identity); remote.head = 'a'.repeat(40);
  await assert.rejects(publish(body, remote, env, identity)); assert.equal(remote.pushes, 1);
});
