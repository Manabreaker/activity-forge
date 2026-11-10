import test from 'node:test';
import assert from 'node:assert/strict';
import { publicationDestination, matchingReceipt } from '../web/client-state.js';

test('an ambiguous publication retains its exact repository across reload and reconnect', () => {
  const frozen = { attempted: true, ticket: { repository: 'example-user/my-custom-calendar' } };
  assert.equal(publicationDestination(frozen, 'activity-calendar'), 'my-custom-calendar');
  assert.equal(publicationDestination({ attempted: false }, 'chosen-name'), 'chosen-name');
  assert.throws(() => publicationDestination({ attempted: true }, 'new-name'));
});
test('a receipt for an older plan cannot mark a new frozen plan as published', () => {
  assert.equal(matchingReceipt({ planId: 'old' }, { plan: { id: 'new' } }), false);
  assert.equal(matchingReceipt({ planId: 'same' }, { plan: { id: 'same' } }), true);
});
