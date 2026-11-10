import { fail, sha256 } from './security.mjs';

export function authorFor(user) {
  if (!Number.isSafeInteger(user.id) || user.id < 1 || !/^[a-z\d](?:[a-z\d-]{0,37}[a-z\d])?$/i.test(user.login || '')) fail('invalid_account', 'Некорректный аккаунт GitHub.', 403);
  return { name: user.login, email: `${user.id}+${user.login}@users.noreply.github.com` };
}

export function validDate(value) {
  return typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value) && Number.isFinite(Date.parse(value)) && new Date(value).toISOString().slice(0, 10) === value;
}

function localStamp(ms, timezone) {
  const fmt = new Intl.DateTimeFormat('en-CA', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' });
  const v = Object.fromEntries(fmt.formatToParts(new Date(ms)).map(p => [p.type, p.value]));
  return `${v.year}-${v.month}-${v.day}T${v.hour}:${v.minute}:${v.second}`;
}

function validateManual(c) {
  if (!Object.hasOwn(c, 'manual')) return null;
  const manual = c.manual, targets = manual?.targets;
  if (!manual || Array.isArray(manual) || !targets || typeof targets !== 'object' || Array.isArray(targets)) fail('invalid_drawing', 'Некорректный пиксельный рисунок.');
  for (const [date, count] of Object.entries(targets)) {
    if (!validDate(date) || date < c.start || date > c.end || !Number.isSafeInteger(count) || count < 0 || count > c.max) fail('invalid_drawing', 'Проверьте даты и насыщенность рисунка.');
  }
  return targets;
}

export async function validatePayload(payload, planId, user) {
  if (typeof payload !== 'string' || payload.length > 1800000 || !/^[a-f\d]{64}$/.test(planId || '') || await sha256(payload) !== planId) fail('plan_changed', 'Контрольная сумма плана не совпала. Зафиксируйте превью ещё раз.');
  let p;
  try { p = JSON.parse(payload); } catch { fail('invalid_plan', 'Некорректный план.'); }
  const c = p.config;
  if (p.version !== 1 || p.id !== '' || !c || !Array.isArray(p.days) || !p.existing || !p.statistics) fail('invalid_plan', 'Неполный план.');
  const author = authorFor(user);
  if (c.author?.name !== author.name || c.author?.email !== author.email) fail('wrong_author', 'План принадлежит другому аккаунту. Обновите превью.', 403);
  if (!validDate(c.start) || !validDate(c.end) || c.start < '1970-01-01' || c.end < c.start || !Number.isInteger(c.max) || c.max < 1 || c.max > 500) fail('invalid_dates', 'Проверьте период и лимит коммитов.');
  const days = (Date.parse(c.end) - Date.parse(c.start)) / 86400000 + 1;
  if (days > 366 || p.days.length !== days || days < 1) fail('invalid_dates', 'Выберите период до 366 дней.');
  const cutoff = Date.parse(c.as_of);
  if (!Number.isFinite(cutoff) || cutoff > Date.now() || typeof c.timezone !== 'string' || c.timezone.length > 80) fail('future_date', 'Нельзя публиковать будущие даты.');
  try { if (c.end > localStamp(cutoff, c.timezone).slice(0, 10)) fail('future_date', 'Нельзя публиковать будущие даты.'); }
  catch { fail('invalid_timezone', 'Некорректный часовой пояс.'); }
  const targets = validateManual(c);
  let previous = -Infinity, planned = 0, existing = 0;
  for (let i = 0; i < days; i++) {
    const d = p.days[i], date = new Date(Date.parse(c.start) + i * 86400000).toISOString().slice(0, 10);
    if (!d || d.date !== date || !Array.isArray(d.timestamps) || ![d.new, d.existing, d.projected, d.target].every(Number.isSafeInteger) || d.target < 0 || d.target > c.max || d.new < 0 || d.existing < 0 || d.existing > 10000000 || d.new !== d.timestamps.length || d.new > Math.max(0, d.target - d.existing) || d.projected !== d.new + d.existing) fail('invalid_counts', 'Количество коммитов расходится с превью.');
    if (targets && d.target !== (targets[date] ?? 0)) fail('invalid_drawing', 'Количество коммитов расходится с рисунком.');
    if (!p.existing.known && d.existing !== 0) fail('invalid_snapshot', 'Неизвестная активность должна быть помечена явно.');
    if (p.existing.known && (p.existing.login !== user.login || p.existing.days?.[date]?.contributionCount !== d.existing)) fail('invalid_snapshot', 'Текущая активность принадлежит другому аккаунту или неполна.');
    for (const stamp of d.timestamps) {
      const ms = typeof stamp === 'string' ? Date.parse(stamp) : NaN;
      if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:\d{2})$/.test(stamp) || !Number.isFinite(ms) || ms <= previous || ms > cutoff || localStamp(ms, c.timezone) !== stamp.slice(0, 19) || stamp.slice(0, 10) !== date) fail('invalid_timestamp', 'Даты коммитов расходятся с планом.');
      previous = ms;
    }
    planned += d.new; existing += d.existing;
    if (planned > 4000) fail('too_many_commits', 'Веб-версия публикует до 4000 коммитов за раз.');
  }
  if (planned < 1 || planned !== p.statistics.planned || existing !== p.statistics.existing || planned + existing !== p.statistics.total) fail('invalid_statistics', 'План пуст или итоговые числа расходятся.');
  return p;
}

export function repositoryName(value) {
  if (typeof value !== 'string' || !/^[a-z\d][a-z\d._-]{0,79}$/i.test(value) || value.endsWith('.git') || value === '.' || value === '..') fail('invalid_repository', 'Имя репозитория: латинские буквы, цифры, точка, дефис и подчёркивание, до 80 символов.');
  return value;
}
