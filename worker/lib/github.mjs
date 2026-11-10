import { Buffer } from 'node:buffer';
import { fail } from './security.mjs';

export class GitHub {
  constructor(token, fetcher = fetch) { this.token = token; this.fetcher = fetcher; }
  async request(path, options = {}) {
    const response = await this.fetcher(`https://api.github.com${path}`, {
      ...options, redirect: 'error', signal: AbortSignal.timeout(25000),
      headers: { Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2026-03-10', 'User-Agent': 'Activity-Forge', Authorization: `Bearer ${this.token}`, ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...options.headers }
    });
    if (response.status === 404 && options.optional) { await response.body?.cancel(); return null; }
    if (!response.ok) {
      await response.body?.cancel();
      if (response.status === 401) fail('github_reauth', 'GitHub отклонил доступ. Подключите аккаунт заново.', 401);
      if (response.status === 403 || response.status === 429) fail('github_permission_or_limit', 'GitHub ограничил запрос: проверьте разрешения приложения или повторите позже.', response.status === 429 ? 429 : 403);
      fail('github_request_failed', 'GitHub не выполнил запрос. Проверьте имя репозитория и повторите с тем же планом.', 400);
    }
    return response.status === 204 ? null : response.json();
  }
  async gitFetch(url, options) {
    if (!url.startsWith('https://github.com/')) fail('invalid_git_url', 'Некорректный адрес Git.', 500);
    const response = await this.fetcher(url, { ...options, redirect: 'error', signal: AbortSignal.timeout(60000), headers: { Authorization: `Basic ${Buffer.from(`x-access-token:${this.token}`).toString('base64')}`, 'User-Agent': 'Activity-Forge', ...options.headers } });
    if (!response.ok) { await response.body?.cancel(); fail('git_request_failed', 'GitHub не подтвердил запрос Git. Повторите проверку с тем же планом.', 400); }
    return response.arrayBuffer();
  }
  async account(appSlug) {
    const user = await this.request('/user');
    const installations = await this.request('/user/installations?per_page=100');
    const installation = installations.installations?.find(x => x.app_slug === appSlug && x.account?.id === user.id && x.account?.type === 'User');
    return { user, installation };
  }
  async calendar(start, end, login) {
    const snapshot = { known: true, login, fetched_at: new Date().toISOString(), start, end, totalContributions: 0, colors: [], days: {} };
    let from = start;
    while (from <= end) {
      const to = end < `${from.slice(0, 4)}-12-31` ? end : `${from.slice(0, 4)}-12-31`;
      const result = await this.request('/graphql', { method: 'POST', body: JSON.stringify({ query: 'query($from:DateTime!,$to:DateTime!){viewer{login contributionsCollection(from:$from,to:$to){contributionCalendar{totalContributions colors weeks{contributionDays{date weekday contributionCount contributionLevel color}}}}}}', variables: { from: from + 'T00:00:00Z', to: to + 'T23:59:59Z' } }) });
      const calendar = result.data?.viewer?.contributionsCollection?.contributionCalendar;
      if (result.errors?.length || !calendar || result.data.viewer.login !== login) fail('calendar_unavailable', 'GitHub не вернул полный календарь. Повторите запрос или явно отключите учёт текущей активности.');
      snapshot.colors = calendar.colors;
      for (const week of calendar.weeks) for (const day of week.contributionDays) if (day.date >= start && day.date <= end) snapshot.days[day.date] = day;
      from = String(Number(from.slice(0, 4)) + 1) + '-01-01';
    }
    for (let ms = Date.parse(start); ms <= Date.parse(end); ms += 86400000) {
      const date = new Date(ms).toISOString().slice(0, 10), day = snapshot.days[date];
      if (!day || !Number.isSafeInteger(day.contributionCount) || day.contributionCount < 0 || day.weekday !== new Date(ms).getUTCDay()) fail('calendar_incomplete', 'GitHub вернул неполные данные. Неизвестные дни не заменены нулями.');
      snapshot.totalContributions += day.contributionCount; delete day.date;
    }
    return snapshot;
  }
}
