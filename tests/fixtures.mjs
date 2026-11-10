import { sha256 } from '../worker/lib/security.mjs';
export const identity = { id: 12345, login: 'example-user' };
export async function samplePlan() {
  const p = { version: 1, id: '', config: { start: '2025-06-01', end: '2025-06-02', as_of: '2025-06-03T00:00:00Z', timezone: 'Europe/Moscow', max: 9, author: { name: identity.login, email: '12345+example-user@users.noreply.github.com' } }, existing: { known: false }, days: [
    { date: '2025-06-01', target: 2, existing: 0, new: 2, projected: 2, timestamps: ['2025-06-01T10:01:02+03:00', '2025-06-01T14:03:04+03:00'] },
    { date: '2025-06-02', target: 1, existing: 0, new: 1, projected: 1, timestamps: ['2025-06-02T12:00:00+03:00'] }
  ], statistics: { planned: 3, existing: 0, total: 3 } };
  const payload = JSON.stringify(p);
  return { payload, planId: await sha256(payload), p };
}
