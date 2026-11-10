import { fail, seal, unseal, sha256 } from './security.mjs';
import { validatePayload, repositoryName } from './plan.mjs';
import { buildPack, pushPack } from './git.mjs';

function checkRepository(repo, user, marker) {
  if (!repo || repo.owner?.id !== user.id || repo.fork || repo.archived || repo.private !== true || repo.description !== marker) fail('unrelated_repository', 'Это существующий или посторонний репозиторий. Выберите новое имя.', 403);
  if (typeof repo.default_branch !== 'string' || !/^[a-z\d._/-]{1,120}$/i.test(repo.default_branch) || repo.default_branch.includes('..')) fail('invalid_branch', 'GitHub вернул неподдерживаемое имя основной ветки.');
}
async function markerFor(env, user, repository, planId) {
  return `Activity Forge decorative history · ${await sha256(`repo-marker\0${env.SESSION_KEY}\0${user.id}\0${repository}\0${planId}`)}`;
}
export async function preparePublication(body, github, env, user) {
  const repository = repositoryName(body.repository);
  const p = await validatePayload(body.payload, body.planId, user);
  const marker = await markerFor(env, user, repository, body.planId);
  const existing = await github.request(`/repos/${user.login}/${repository}`, { optional: true });
  if (existing) checkRepository(existing, user, marker);
  const ticket = await seal({ userId: user.id, login: user.login, repository, planId: body.planId, count: p.statistics.planned, marker }, env.SESSION_KEY, 'publish', 86400);
  return { ticket, repository: `${user.login}/${repository}`, commits: p.statistics.planned, private: true, branch: 'default', planId: body.planId };
}
export async function publish(body, github, env, user) {
  if (body.confirmed !== true) fail('confirmation_required', 'Публикация требует явного подтверждения.', 403);
  const ticket = await unseal(body.ticket, env.SESSION_KEY, 'publish');
  if (ticket.userId !== user.id || ticket.login !== user.login || ticket.planId !== body.planId) fail('wrong_ticket', 'Подтверждение не принадлежит этому аккаунту или плану.', 403);
  const p = await validatePayload(body.payload, body.planId, user);
  if (p.statistics.planned !== ticket.count) fail('plan_changed', 'Количество коммитов изменилось.');
  const repository = repositoryName(ticket.repository), path = `/repos/${user.login}/${repository}`;
  let repo = await github.request(path, { optional: true });
  if (!repo) {
    try { repo = await github.request('/user/repos', { method: 'POST', body: JSON.stringify({ name: repository, description: ticket.marker, private: true, auto_init: false }) }); }
    catch (error) { repo = await github.request(path, { optional: true }); if (!repo) throw error; }
  }
  checkRepository(repo, user, ticket.marker);
  const built = buildPack(p, body.planId, user);
  await pushPack(github, user.login, repository, repo.default_branch, built);
  const fresh = await github.request(path);
  checkRepository(fresh, user, ticket.marker);
  if (fresh.default_branch !== repo.default_branch) fail('branch_changed', 'Основная ветка изменилась. Проверьте репозиторий.', 403);
  const ref = await github.request(`${path}/git/ref/heads/${encodeURIComponent(repo.default_branch)}`);
  if (ref.object?.sha !== built.head) fail('head_mismatch', 'GitHub ещё не подтвердил точную историю. Повторите проверку с тем же планом.', 500);
  return { verified: true, url: `https://github.com/${user.login}/${repository}`, repository: `${user.login}/${repository}`, private: true, branch: repo.default_branch, commits: built.count, head: built.head, planId: body.planId, verifiedAt: new Date().toISOString() };
}
