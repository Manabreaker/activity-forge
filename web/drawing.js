export const BRUSH_COUNTS = [0, 1, 3, 6, 10];
const validDate = d => /^\d{4}-\d{2}-\d{2}$/.test(d) && Number.isFinite(Date.parse(d)) && new Date(d).toISOString().slice(0, 10) === d;
const cleanCells = cells => Object.fromEntries(Object.entries(cells && typeof cells === 'object' ? cells : {}).filter(([date, count]) => validDate(date) && count > 0 && BRUSH_COUNTS.includes(count)));

export class Drawing {
  constructor(cells = {}) { this.cells = cleanCells(cells); this.past = []; this.future = []; this.before = null; this.locked = false; }
  get canUndo() { return this.past.length > 0; }
  get canRedo() { return this.future.length > 0; }
  begin() { if (this.locked) return; this.end(); this.before = { ...this.cells }; }
  paint(date, count, existing = 0) {
    if (this.locked) return false;
    if (!validDate(date) || !BRUSH_COUNTS.includes(count)) return false;
    if (count && existing + count > 500) throw new Error('На этот день уже приходится слишком много contributions: предел цели — 500.');
    if ((this.cells[date] || 0) === count) return false;
    if (this.before === null) this.begin();
    if (count) this.cells[date] = count; else delete this.cells[date];
    return true;
  }
  end() {
    if (this.before === null) return false;
    const changed = JSON.stringify(Object.entries(this.before).sort()) !== JSON.stringify(Object.entries(this.cells).sort());
    if (changed) { this.past.push(this.before); if (this.past.length > 100) this.past.shift(); this.future = []; }
    this.before = null; return changed;
  }
  clear() { if (this.locked) return false; this.begin(); this.cells = {}; return this.end(); }
  undo() { if (this.locked) return false; this.end(); if (!this.canUndo) return false; this.future.push(this.cells); this.cells = this.past.pop(); return true; }
  redo() { if (this.locked) return false; this.end(); if (!this.canRedo) return false; this.past.push(this.cells); this.cells = this.future.pop(); return true; }
}

export function drawingConfig(config, snapshot, cells) {
  const c = structuredClone(config), targets = {};
  for (const [date, count] of Object.entries(cleanCells(cells))) {
    if (date < c.start || date > c.end) continue;
    const existing = snapshot.known ? snapshot.days?.[date]?.contributionCount || 0 : 0;
    if (existing + count > 500) throw new Error(`На ${date} цель превышает 500 contributions. Сотри эту клетку или обнови текущую активность.`);
    targets[date] = existing + count;
  }
  c.max = 500; c.manual = { targets }; return c;
}

export function brushLevel(count) { return count <= 0 ? 0 : Math.min(4, 1 + [1, 3, 6].filter(n => count > n).length); }

export function calendarLayout(start, length) {
  const offset = new Date(start).getUTCDay(), weeks = Math.ceil((offset + length) / 7);
  return { offset, length, weeks, step: 13, cell: 10, left: 29, top: 22, width: 29 + weeks * 13 + 5 };
}

export function cellIndex({ x, y }, layout) {
  const i = x * 7 + y - layout.offset;
  return x < 0 || x >= layout.weeks || y < 0 || y > 6 || i < 0 || i >= layout.length ? -1 : i;
}

export function gridLine(a, b) {
  const cells = [], dx = Math.abs(b.x - a.x), dy = -Math.abs(b.y - a.y), sx = a.x < b.x ? 1 : -1, sy = a.y < b.y ? 1 : -1;
  let { x, y } = a, error = dx + dy;
  for (;;) {
    cells.push({ x, y }); if (x === b.x && y === b.y) return cells;
    const e = 2 * error;
    if (e >= dy) { error += dy; x += sx; }
    if (e <= dx) { error += dx; y += sy; }
  }
}

export function restoreEditor(frozen, fallback) {
  if (frozen.editor) return { config: structuredClone(frozen.editor.config), mode: frozen.editor.mode === 'draw' ? 'draw' : 'generate', cells: cleanCells(frozen.editor.cells) };
  const c = frozen.plan.config;
  if (!c.manual) return { config: structuredClone(c), mode: 'generate', cells: {} };
  const config = structuredClone(fallback);
  for (const field of ['start', 'end', 'timezone', 'seed', 'as_of', 'author']) if (c[field] !== undefined) config[field] = c[field];
  return { config, mode: 'draw', cells: cleanCells(Object.fromEntries(frozen.plan.days.map(d => [d.date, d.new]))) };
}
