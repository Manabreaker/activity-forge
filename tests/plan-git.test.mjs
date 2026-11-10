import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { validatePayload } from '../worker/lib/plan.mjs';
import { buildPack, pktLine, readPackets } from '../worker/lib/git.mjs';
import { samplePlan, identity } from './fixtures.mjs';
import { sha256 } from '../worker/lib/security.mjs';

test('server accepts exactly the frozen plan and rejects tampering, future dates and other authors', async () => {
  const f = await samplePlan();
  assert.equal((await validatePayload(f.payload, f.planId, identity)).statistics.planned, 3);
  await assert.rejects(validatePayload(f.payload + ' ', f.planId, identity));
  await assert.rejects(validatePayload(f.payload, f.planId, { ...identity, id: 999 }));
  for (const mutate of [
    p => p.days[0].timestamps.push(p.days[0].timestamps[0]),
    p => p.days[0].timestamps[0] = '2099-06-01T10:00:00+03:00',
    p => p.days[1].date = p.days[0].date,
    p => p.days[0].projected = 900,
    p => p.days[0].timestamps[0] = '2025-06-01T10:01:02-03:00'
  ]) {
    const p = structuredClone(f.p); mutate(p);
    const payload = JSON.stringify(p);
    await assert.rejects(validatePayload(payload, await sha256(payload), identity));
  }
});

test('in-memory pack imports into real Git with exact dates, identity, manifest and commit count', async () => {
  const f = await samplePlan();
  const plan = await validatePayload(f.payload, f.planId, identity);
  const result = buildPack(plan, f.planId, identity);
  const dir = await mkdtemp(join(tmpdir(), 'forge-pack-'));
  try {
    execFileSync('git', ['init', '--bare', dir], { stdio: 'pipe' });
    execFileSync('git', ['-C', dir, 'index-pack', '--stdin'], { input: result.pack, stdio: 'pipe' });
    execFileSync('git', ['-C', dir, 'update-ref', 'refs/heads/main', result.head]);
    const log = execFileSync('git', ['-C', dir, 'log', '--reverse', '--format=%aI|%cI|%ae', result.head], { encoding: 'utf8' }).trim().split('\n');
    const dates = plan.days.flatMap(d => d.timestamps);
    assert.deepEqual(log, dates.map(d => `${d}|${d}|12345+example-user@users.noreply.github.com`));
    const manifest = JSON.parse(execFileSync('git', ['-C', dir, 'show', `${result.head}:.activityforge.json`], { encoding: 'utf8' }));
    assert.equal(manifest.plan_id, f.planId); assert.equal(manifest.commits, 3);
    execFileSync('git', ['-C', dir, 'fsck', '--strict'], { stdio: 'pipe' });
    assert.equal(buildPack(plan, f.planId, identity).head, result.head);
  } finally { await rm(dir, { recursive: true, force: true }); }
});

test('Git packet framing checks lengths and rejects truncated responses', () => {
  assert.deepEqual(readPackets(Buffer.concat([pktLine('unpack ok\n'), pktLine('ok refs/heads/main\n'), Buffer.from('0000')])), ['unpack ok\n', 'ok refs/heads/main\n']);
  assert.throws(() => readPackets(Buffer.from('0009oops')));
});
