import test from 'node:test';
import assert from 'node:assert/strict';
import { Drawing, drawingConfig, brushLevel, gridLine, calendarLayout, cellIndex, restoreEditor } from '../web/drawing.js';

test('one drag is one undo step; erasing and clearing preserve undo and invalidate redo', () => {
  const d = new Drawing();
  d.begin(); d.paint('2025-06-01', 3); d.paint('2025-06-02', 3); d.paint('2025-06-02', 3); d.end();
  assert.deepEqual(d.cells, { '2025-06-01': 3, '2025-06-02': 3 });
  d.undo(); assert.deepEqual(d.cells, {});
  d.redo(); assert.equal(d.cells['2025-06-02'], 3);
  d.begin(); d.paint('2025-06-01', 0); d.end();
  assert.deepEqual(d.cells, { '2025-06-02': 3 });
  d.clear(); assert.deepEqual(d.cells, {});
  d.undo(); assert.deepEqual(d.cells, { '2025-06-02': 3 });
  d.begin(); d.paint('2025-06-03', 6); d.end();
  assert.equal(d.canRedo, false);
});

test('manual config adds baseline only to painted dates and never mutates automatic settings', () => {
  const c = { start: '2025-06-01', end: '2025-06-03', max: 9, author: { name: 'example' } };
  const snapshot = { known: true, days: { '2025-06-01': { contributionCount: 8 }, '2025-06-02': { contributionCount: 900 } } };
  const config = drawingConfig(c, snapshot, { '2025-06-01': 6, '2025-05-01': 10 });
  assert.deepEqual(config.manual, { targets: { '2025-06-01': 14 } });
  assert.equal(config.max, 500); assert.equal(c.max, 9); assert.equal(c.manual, undefined);
  assert.deepEqual(drawingConfig(c, snapshot, {}).manual, { targets: {} });
  assert.throws(() => drawingConfig(c, snapshot, { '2025-06-02': 1 }));
});

test('palette shades remain stable even when all painted cells use one shade', () => {
  assert.deepEqual([0, 1, 3, 6, 10].map(brushLevel), [0, 1, 2, 3, 4]);
});

test('fast strokes interpolate diagonals and skip cells outside partial weeks', () => {
  const layout = calendarLayout('2025-06-04', 4);
  assert.equal(cellIndex({ x: 0, y: 2 }, layout), -1);
  assert.equal(cellIndex({ x: 0, y: 3 }, layout), 0);
  assert.equal(cellIndex({ x: 0, y: 6 }, layout), 3);
  assert.equal(cellIndex({ x: 1, y: 0 }, layout), -1);
  assert.deepEqual(gridLine({ x: 1, y: 1 }, { x: 4, y: 4 }), [{ x: 1, y: 1 }, { x: 2, y: 2 }, { x: 3, y: 3 }, { x: 4, y: 4 }]);
  assert.equal(gridLine({ x: 0, y: 1 }, { x: 10, y: 1 }).length, 11);
});

test('restoring a frozen drawing preserves the base automatic config and filters corrupt local cells', () => {
  const base = { start: '2025-06-01', end: '2025-06-02', max: 9 };
  const editor = { config: base, mode: 'draw', cells: { '2025-06-01': 6 } };
  const f = { editor, plan: { config: { ...base, max: 500, manual: { targets: { '2025-06-01': 14 } } } } };
  const restored = restoreEditor(f, base);
  assert.equal(restored.config.max, 9); assert.equal(restored.config.manual, undefined);
  assert.equal(restored.mode, 'draw'); assert.deepEqual(restored.cells, editor.cells);
  const clean = new Drawing({ '2025-06-01': 3, bad: 10, '2025-06-02': -1, '2025-06-03': 999 });
  assert.deepEqual(clean.cells, { '2025-06-01': 3 });
});

test('locking during a captured stroke prevents any further painting, erasing or history edits', () => {
  const d = new Drawing({ '2025-06-01': 3 });
  d.begin(); d.paint('2025-06-02', 6); d.end(); d.begin();
  d.locked = true;
  assert.equal(d.paint('2025-06-03', 10), false);
  assert.equal(d.paint('2025-06-01', 0), false);
  assert.equal(d.clear(), false); assert.equal(d.undo(), false); assert.equal(d.redo(), false);
  assert.deepEqual(d.cells, { '2025-06-01': 3, '2025-06-02': 6 });
  d.locked = false; d.end(); d.undo();
  assert.deepEqual(d.cells, { '2025-06-01': 3 });
});
