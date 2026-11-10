import test from 'node:test';
import assert from 'node:assert/strict';
import { currentDayStyle } from '../web/calendar-colors.js';

const dark = ['#20292a', '#204b38', '#2e7951', '#58a773', '#a2dfba'];
const light = ['#e9eee7', '#bbdfae', '#74b676', '#3f8b50', '#246239'];

test('empty GitHub days use the selected theme even when the API returns light-theme white', () => {
  const entry = { contributionLevel: 'NONE', color: '#ebedf0' };
  assert.deepEqual(currentDayStyle(0, entry, dark, 0), { level: 0, fill: dark[0] });
  assert.deepEqual(currentDayStyle(0, entry, light, 0), { level: 0, fill: light[0] });
});

test('active GitHub days retain their API level and valid color', () => {
  const entry = { contributionLevel: 'THIRD_QUARTILE', color: '#30a14e' };
  assert.deepEqual(currentDayStyle(5, entry, dark, 1), { level: 3, fill: '#30a14e' });
  assert.deepEqual(currentDayStyle(5, entry, light, 1), { level: 3, fill: '#30a14e' });
});

test('missing and invalid API colors fall back to the current theme palette', () => {
  assert.deepEqual(currentDayStyle(1, { contributionLevel: 'FIRST_QUARTILE', color: 'url(example)' }, dark, 4), { level: 1, fill: dark[1] });
  assert.deepEqual(currentDayStyle(1, undefined, dark, 2), { level: 2, fill: dark[2] });
  assert.deepEqual(currentDayStyle(0, { contributionLevel: 'FOURTH_QUARTILE', color: '#ffffff' }, dark, 4), { level: 0, fill: dark[0] });
});
