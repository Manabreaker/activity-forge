import test from 'node:test';
import assert from 'node:assert/strict';
import { samplePlan, identity } from './fixtures.mjs';
import { validatePayload } from '../worker/lib/plan.mjs';
import { sha256 } from '../worker/lib/security.mjs';

test('manual publication accepts exact targets and rejects malformed or inconsistent drawings', async () => {
  const { p } = await samplePlan();
  p.config.manual = { targets: { '2025-06-01': 2, '2025-06-02': 1 } };
  const validate = async plan => { const payload = JSON.stringify(plan); return validatePayload(payload, await sha256(payload), identity); };
  assert.equal((await validate(p)).statistics.planned, 3);
  for (const manual of [null, [], {}, { targets: null }, { targets: [] }, { targets: { '2025-06-01': -1 } }, { targets: { '2025-06-01': 10 } }, { targets: { '2025-06-01': 2, '2025-06-02': 1, '2025-05-31': 1 } }, { targets: { '2025-06-01': 1, '2025-06-02': 1 } }, { targets: {} }]) {
    const edited = structuredClone(p); edited.config.manual = manual;
    await assert.rejects(validate(edited), 'invalid manual drawing accepted: ' + JSON.stringify(manual));
  }
});
