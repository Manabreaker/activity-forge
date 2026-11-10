import { createHash } from 'node:crypto';
import { deflateSync } from 'node:zlib';
import { Buffer } from 'node:buffer';
import { authorFor } from './plan.mjs';
import { fail } from './security.mjs';

function object(kind, body) {
  const data = Buffer.isBuffer(body) ? body : Buffer.from(body);
  const hash = createHash('sha1').update(`${kind} ${data.length}\0`).update(data).digest('hex');
  const type = { commit: 1, tree: 2, blob: 3 }[kind];
  let n = data.length;
  const header = [(type << 4) | (n & 15)]; n >>>= 4;
  while (n) { header[header.length - 1] |= 128; header.push(n & 127); n >>>= 7; }
  return { hash, packed: Buffer.concat([Buffer.from(header), deflateSync(data)]) };
}

export function buildPack(plan, planId, user) {
  const author = authorFor(user), total = plan.statistics.planned;
  const manifest = object('blob', JSON.stringify({ version: 1, tool: 'activity-forge-web', purpose: 'decorative contribution history', plan_id: planId, commits: total, owner_id: user.id, start: plan.config.start, end: plan.config.end }) + '\n');
  const readme = object('blob', `# Decorative activity calendar\n\nThis repository contains intentionally generated, decorative Git history.\nIt does not represent work on a software product.\n\nCreated with [Activity Forge](https://github.com/Manabreaker/activity-forge).\n\nPlan: \`${planId}\`\nCommits: ${total}\n`);
  const tree = object('tree', Buffer.concat([
    Buffer.from('100644 .activityforge.json\0'), Buffer.from(manifest.hash, 'hex'),
    Buffer.from('100644 README.md\0'), Buffer.from(readme.hash, 'hex')
  ]));
  const objects = [manifest, readme, tree];
  let head = '', index = 0;
  for (const day of plan.days) for (const stamp of day.timestamps) {
    const seconds = Math.floor(Date.parse(stamp) / 1000), zone = stamp.endsWith('Z') ? '+0000' : stamp.slice(-6).replace(':', '');
    const identity = `${author.name} <${author.email}> ${seconds} ${zone}`;
    const commit = object('commit', `tree ${tree.hash}\n${head ? `parent ${head}\n` : ''}author ${identity}\ncommitter ${identity}\n\nDecorative activity ${++index}/${total}\n\nActivity-Forge-Plan: ${planId}\n`);
    objects.push(commit); head = commit.hash;
  }
  const header = Buffer.alloc(12); header.write('PACK'); header.writeUInt32BE(2, 4); header.writeUInt32BE(objects.length, 8);
  const body = Buffer.concat([header, ...objects.map(o => o.packed)]);
  return { head, count: index, pack: Buffer.concat([body, createHash('sha1').update(body).digest()]) };
}

export function pktLine(value) {
  const body = Buffer.isBuffer(value) ? value : Buffer.from(value);
  const length = body.length + 4;
  if (length > 65520) throw new Error('git packet too large');
  return Buffer.concat([Buffer.from(length.toString(16).padStart(4, '0')), body]);
}

export function readPackets(data) {
  const body = Buffer.from(data), result = [];
  for (let i = 0; i < body.length;) {
    const raw = body.subarray(i, i + 4).toString();
    if (!/^[\da-f]{4}$/i.test(raw)) throw new Error('invalid git packet');
    const length = parseInt(raw, 16); i += 4;
    if (length === 0) continue;
    if (length < 4 || length > 65520 || i + length - 4 > body.length) throw new Error('truncated git packet');
    result.push(body.subarray(i, i + length - 4).toString('utf8')); i += length - 4;
  }
  return result;
}

export async function pushPack(github, owner, name, branch, built) {
  const base = `https://github.com/${encodeURIComponent(owner)}/${encodeURIComponent(name)}.git`;
  const advertised = await github.gitFetch(base + '/info/refs?service=git-receive-pack', { method: 'GET' });
  const packets = readPackets(advertised);
  const refs = packets.filter(v => /^[a-f\d]{40} refs\//.test(v));
  if (refs.length) {
    const own = refs.filter(v => v.split(/[\0\n ]/)[1] === `refs/heads/${branch}`);
    if (refs.length === 1 && own.length === 1 && own[0].startsWith(built.head + ' ')) return;
    fail('remote_not_empty', 'В репозитории уже есть другая история. Публикация остановлена.', 403);
  }
  if (!packets.some(v => v.includes('report-status'))) fail('git_protocol', 'GitHub не подтвердил поддержку безопасной отправки.', 500);
  const command = pktLine(`${'0'.repeat(40)} ${built.head} refs/heads/${branch}\0report-status\n`);
  const response = await github.gitFetch(base + '/git-receive-pack', { method: 'POST', headers: { 'Content-Type': 'application/x-git-receive-pack-request' }, body: Buffer.concat([command, Buffer.from('0000'), built.pack]) });
  const status = readPackets(response);
  if (!status.includes('unpack ok\n') || !status.includes(`ok refs/heads/${branch}\n`)) fail('push_unconfirmed', 'GitHub не подтвердил отправку. Проверьте результат повторно с тем же планом.', 500);
}
