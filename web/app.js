import { API_ORIGIN } from './config.js';
import { publicationDestination, matchingReceipt } from './client-state.js';
import { Drawing, BRUSH_COUNTS, drawingConfig, brushLevel, calendarLayout, cellIndex, gridLine, restoreEditor } from './drawing.js';
const $ = id => document.getElementById(id);
const num = n => Number(n).toLocaleString('ru-RU');
const esc = value => String(value).replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
const state = { config: null, profile: 'bursts', result: null, snapshot: { known: false }, account: null, mode: 'preview', editorMode: 'generate', drawing: new Drawing(), brush: 10, tool: 'brush', frozen: null, ticket: null, publishing: false, ready: false, baselineLoading: false, epoch: 0 };
let configReady = false, generationTimer;
let stroke = null;
let lastPreview = null;
const store = {
  get(key, fallback = null) { try { return JSON.parse(localStorage.getItem('af.' + key)) ?? fallback; } catch { return fallback; } },
  set(key, value) { try { localStorage.setItem('af.' + key, JSON.stringify(value)); } catch { notice('Браузер не разрешает сохранять настройки. Скачай понравившийся план перед закрытием вкладки.'); } },
  remove(key) { try { localStorage.removeItem('af.' + key); } catch {} }
};
function session() { try { return sessionStorage.getItem('af.session'); } catch { return null; } }
function notice(message, kind = '') { $('notice').hidden = !message; $('notice').textContent = message; $('notice').className = 'notice ' + kind; }
function generator(request) {
  const result = JSON.parse(window.activityForge(JSON.stringify(request)));
  if (result.error) throw new Error(result.error);
  return result;
}
function key(name) { return name + '.' + (state.account?.id ?? 'anonymous'); }
async function api(path, body) {
  const response = await fetch(API_ORIGIN + path, { method: body === undefined ? 'GET' : 'POST', mode: 'cors', credentials: 'omit', cache: 'no-store', signal: AbortSignal.timeout(path === '/api/publish' ? 180000 : 70000), headers: { ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...(session() ? { Authorization: 'Bearer ' + session() } : {}) }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  const result = await response.json();
    if (!response.ok) {
      if (response.status === 401) {
        try { sessionStorage.removeItem('af.session'); } catch {}
        state.authExpired = true; $('connect').hidden = false; $('connect').textContent = 'Подключить GitHub снова ↗'; $('refresh-account').hidden = true; $('prepare').disabled = true;
      }
      const error = new Error(result.message || 'Не удалось выполнить запрос.'); error.code = result.code; throw error;
    }
  return result;
}
function setControls() {
  const c = state.config;
  for (const id of ['start', 'end', 'intensity', 'randomness', 'timezone', 'seed']) $(id).value = c[id];
  $('weekends').value = c.weekends.mode;
  $('weekend-probability').value = Math.round(c.weekends.saturday.probability * 100);
  $('weekend-multiplier').value = c.weekends.saturday.multiplier;
  $('bursts').checked = c.effects.work_bursts; $('rests').checked = c.effects.rest_periods;
  $('monthly').checked = c.effects.monthly_variation; $('streaks').checked = c.effects.streaks;
  $('vacations').value = c.effects.vacation_count; $('trend').value = c.effects.trend;
  $('intensity-value').textContent = c.intensity + ' / 10'; $('randomness-value').textContent = c.randomness + '%';
  $('weekend-custom').hidden = c.weekends.mode !== 'custom';
  document.querySelectorAll('[data-profile]').forEach(b => b.classList.toggle('selected', b.dataset.profile === state.profile));
}
function readControls() {
  const c = state.config;
  for (const id of ['start', 'end', 'timezone']) c[id] = $(id).value;
  for (const id of ['intensity', 'randomness', 'seed']) c[id] = Number($(id).value);
  c.weekends.mode = $('weekends').value;
  for (const day of ['saturday', 'sunday']) { c.weekends[day].probability = Number($('weekend-probability').value) / 100; c.weekends[day].multiplier = Number($('weekend-multiplier').value); }
  c.effects.work_bursts = $('bursts').checked; c.effects.rest_periods = $('rests').checked;
  c.effects.monthly_variation = $('monthly').checked; c.effects.streaks = $('streaks').checked;
  c.effects.vacation_count = Number($('vacations').value); c.effects.trend = $('trend').value;
  c.author = state.account?.author || { name: '', email: '' };
  $('intensity-value').textContent = c.intensity + ' / 10'; $('randomness-value').textContent = c.randomness + '%';
  $('weekend-custom').hidden = c.weekends.mode !== 'custom';
}
function persistSettings() {
  const c = structuredClone(state.config); c.author = { name: '', email: '' };
  store.set('settings', { config: c, profile: state.profile });
}
function persistDrawing() { store.set('drawing', { mode: state.editorMode, cells: state.drawing.cells, brush: state.brush }); }
function editorSnapshot() { return { config: structuredClone(state.config), mode: state.editorMode, cells: { ...state.drawing.cells } }; }
function restoreFrozen(saved) {
  finishStroke();
  const editor = restoreEditor(saved, state.config);
  state.config = editor.config; state.editorMode = editor.mode;
  if (editor.mode === 'draw') state.drawing = new Drawing(editor.cells);
  state.snapshot = saved.plan.existing; state.result = saved; setControls(); setFrozen(saved); persistDrawing();
}
function generate() {
  if (!state.ready || state.frozen) return;
  try {
    if (!$('settings').checkValidity()) throw new Error('Проверь даты и настройки перед сохранением плана.');
    readControls();
    if ((Date.parse(state.config.end) - Date.parse(state.config.start)) / 86400000 > 365) throw new Error('Выбери период до 366 дней.');
    state.config.as_of = new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
    const config = state.editorMode === 'draw' ? drawingConfig(state.config, state.snapshot, state.drawing.cells) : state.config;
    state.result = generator({ action: 'generate', config, existing: state.snapshot });
    lastPreview = state.result.plan;
    persistSettings(); render(); notice(''); return true;
  } catch (error) { state.result = null; $('freeze').disabled = true; $('export').disabled = true; notice(error.message, 'error'); return false; }
}
function schedule() { $('freeze').disabled = true; $('export').disabled = true; clearTimeout(generationTimer); generationTimer = setTimeout(generate, 120); }
function flushPlan() { finishStroke(); clearTimeout(generationTimer); return state.frozen ? true : generate(); }
function plan() { return state.frozen?.plan || state.result?.plan || lastPreview; }
function countFor(day) { return state.mode === 'current' ? day.existing : state.mode === 'diff' ? day.new : day.projected; }
function levelFor(n, positives) {
  if (!n) return 0;
  for (let i = 1; i <= 3; i++) if (n <= positives[Math.min(positives.length - 1, Math.floor(positives.length * i / 4))]) return i;
  return 4;
}
function graphSVG(p, exported = false) {
  const { offset, step, cell, left, top, width } = calendarLayout(p.config.start, p.days.length);
  const positives = p.days.map(countFor).filter(n => n > 0).sort((a, b) => a - b);
  const colors = [0, 1, 2, 3, 4].map(n => getComputedStyle(document.documentElement).getPropertyValue('--l' + n).trim());
  const muted = getComputedStyle(document.documentElement).getPropertyValue('--muted').trim();
  let svg = `<svg xmlns="http://www.w3.org/2000/svg" role="img" aria-label="Календарь активности" viewBox="0 0 ${width} 119"${exported ? ` width="${width * 2}" height="238"` : ''}>`;
  if (exported) svg += `<rect width="${width}" height="119" fill="${getComputedStyle(document.documentElement).getPropertyValue('--panel').trim()}"/>`;
  for (const [row, label] of [[1, 'Пн'], [3, 'Ср'], [5, 'Пт']]) svg += `<text x="0" y="${top + row * step + 8}" fill="${muted}" font-family="sans-serif" font-size="9">${label}</text>`;
  let lastMonth = '', lastLabel = -5;
  for (let i = 0; i < p.days.length; i++) {
    const d = p.days[i], column = Math.floor((offset + i) / 7), row = (offset + i) % 7;
    const month = d.date.slice(0, 7);
    if (month !== lastMonth && column - lastLabel > 2) {
      const text = new Intl.DateTimeFormat('ru', { month: 'short', timeZone: 'UTC' }).format(new Date(d.date)).replace('.', '');
      svg += `<text x="${left + column * step}" y="10" fill="${muted}" font-family="sans-serif" font-size="9">${esc(text)}</text>`;
      lastLabel = column;
    }
    lastMonth = month;
    let level = p.config.manual && state.mode !== 'current' ? brushLevel(countFor(d)) : levelFor(countFor(d), positives), fill = colors[level];
    if (state.mode === 'current' && p.existing.known) {
      const current = p.existing.days[d.date];
      const actual = ['NONE', 'FIRST_QUARTILE', 'SECOND_QUARTILE', 'THIRD_QUARTILE', 'FOURTH_QUARTILE'].indexOf(current?.contributionLevel);
      if (actual >= 0) level = actual;
      fill = /^#[a-f\d]{6}$/i.test(current?.color || '') ? current.color : colors[level];
    }
    const title = `${d.date} · уже есть: ${p.existing.known ? d.existing : 'неизвестно'} · добавится: ${d.new} · итог: ${d.projected}`;
    svg += `<rect class="day level-${level}" ${exported ? '' : `tabindex="0" role="button" data-day="${i}" aria-label="${esc(title)}"`} x="${left + column * step}" y="${top + row * step}" width="${cell}" height="${cell}" fill="${fill}" rx="2"><title>${esc(title)}</title></rect>`;
  }
  return svg + '</svg>';
}
function render() {
  const p = plan(); if (!p) return;
  $('calendar').innerHTML = graphSVG(p);
  $('period-label').textContent = p.config.start.replaceAll('-', '.') + ' — ' + p.config.end.replaceAll('-', '.');
  const total = state.mode === 'current' ? p.statistics.existing : state.mode === 'diff' ? p.statistics.planned : p.statistics.total;
  $('calendar-total').textContent = state.mode === 'current' && !p.existing.known ? 'Текущая активность ещё не загружена' : num(total) + (state.mode === 'diff' ? ' новых коммитов' : ' contributions за период');
  $('baseline-status').textContent = p.existing.known ? `Учтено ${num(p.statistics.existing)} contributions · @${p.existing.login}` : 'Существующая активность неизвестна · показан прогноз добавлений';
  $('metric-planned').textContent = '+' + num(p.statistics.planned); $('metric-days').textContent = num(p.statistics.active_days); $('metric-streak').textContent = num(p.statistics.longest_streak); $('metric-average').textContent = p.statistics.average_active.toFixed(1);
  $('metric-percent').textContent = Math.round(p.statistics.active_days / p.days.length * 100) + '% дней за период';
  const chart = $('monthly-chart'); chart.replaceChildren();
  const maximum = Math.max(1, ...p.statistics.monthly.map(m => m.total));
  for (const m of p.statistics.monthly) {
    const column = document.createElement('div'), bar = document.createElement('i'), label = document.createElement('small'); column.className = 'month-column';
    bar.style.height = (45 * m.total / maximum) + 'px'; label.textContent = new Intl.DateTimeFormat('ru', { month: 'short', timeZone: 'UTC' }).format(new Date(m.month + '-01')).replace('.', '');
    column.title = `${m.month}: ${m.total}, добавится ${m.planned}`; column.append(bar, label); chart.append(column);
  }
  $('freeze').disabled = (!state.frozen && !state.result) || p.statistics.planned === 0 || state.baselineLoading;
  $('export').disabled = !state.frozen && !state.result;
  $('plan-badge').innerHTML = state.frozen ? '<span class="status-dot"></span> План зафиксирован' : '<span class="status-dot"></span> Живое превью';
  $('prepare').disabled = !state.frozen || !state.account || state.authExpired || !state.account.installation.installed || state.baselineLoading || ($('use-existing').checked && !p.existing.known);
  updateEditor();
}

function updateEditor() {
  const draw = state.editorMode === 'draw', locked = !!state.frozen;
  $('drawing-tools').hidden = !draw;
  document.querySelector('.controls').classList.toggle('drawing-settings', draw);
  document.querySelectorAll('#settings input, #settings select, [data-profile], #reset, #regenerate').forEach(x => x.disabled = locked);
  if (draw) {
    for (const id of ['intensity', 'randomness', 'weekends', 'weekend-probability', 'weekend-multiplier', 'bursts', 'rests', 'monthly', 'streaks', 'vacations', 'trend', 'seed', 'regenerate']) $(id).disabled = true;
    document.querySelectorAll('[data-profile]').forEach(b => b.disabled = true);
  }
  document.querySelectorAll('[data-editor]').forEach(b => { const selected = b.dataset.editor === state.editorMode; b.classList.toggle('active', selected); b.setAttribute('aria-pressed', selected); b.disabled = locked; });
  document.querySelectorAll('[data-mode]').forEach(b => { const selected = b.dataset.mode === state.mode; b.classList.toggle('active', selected); b.setAttribute('aria-pressed', selected); });
  document.querySelectorAll('[data-brush]').forEach(b => { const selected = state.tool === 'brush' && Number(b.dataset.brush) === state.brush; b.classList.toggle('selected', selected); b.setAttribute('aria-pressed', selected); b.disabled = locked; });
  $('draw-hand').classList.toggle('selected', state.tool === 'hand'); $('draw-hand').setAttribute('aria-pressed', state.tool === 'hand');
  const readOnly = locked || state.mode === 'current' || state.baselineLoading;
  $('draw-undo').disabled = readOnly || !state.drawing.canUndo; $('draw-redo').disabled = readOnly || !state.drawing.canRedo;
  $('draw-clear').disabled = readOnly || Object.keys(state.drawing.cells).length === 0;
  const shell = document.querySelector('.graph-shell');
  shell.classList.toggle('drawing', draw && !locked && state.mode !== 'current'); shell.classList.toggle('hand', state.tool === 'hand');
  $('drawing-hint').textContent = locked ? 'Рисунок зафиксирован. Чтобы рисовать дальше, нажми «Вернуться к настройке».' : state.mode === 'current' ? 'Сейчас показана реальная история GitHub. Для рисования выбери «Добавится» или «Результат».' : 'Рисуй мышью или пальцем. Цифра — новых коммитов за день. Ластик убирает только рисунок. Для прокрутки выбери «Перемещение». Клавиатура: стрелки и пробел.';
}

function canDraw() { return state.ready && state.editorMode === 'draw' && !state.frozen && !state.baselineLoading && state.mode !== 'current'; }
function pointerCell(event) {
  const p = plan(), svg = $('calendar').querySelector('svg'), matrix = svg?.getScreenCTM();
  if (!p || !matrix) return null;
  const point = new DOMPoint(event.clientX, event.clientY).matrixTransform(matrix.inverse());
  const l = calendarLayout(p.config.start, p.days.length);
  if (point.x < l.left || point.x >= l.left + l.weeks * l.step || point.y < l.top || point.y >= l.top + 7 * l.step) return null;
  return { x: Math.floor((point.x - l.left) / l.step), y: Math.floor((point.y - l.top) / l.step) };
}
function paintCells(points) {
  const p = plan(); if (!p) return;
  const layout = calendarLayout(p.config.start, p.days.length);
  for (const point of points) {
    const i = cellIndex(point, layout); if (i < 0) continue;
    const d = p.days[i], existing = state.snapshot.known ? state.snapshot.days?.[d.date]?.contributionCount || 0 : 0;
    try {
      if (!state.drawing.paint(d.date, state.brush, existing)) continue;
      const count = state.mode === 'preview' ? existing + state.brush : state.brush;
      const rect = $('calendar').querySelector(`[data-day="${i}"]`);
      rect?.setAttribute('fill', getComputedStyle(document.documentElement).getPropertyValue('--l' + brushLevel(count)).trim());
      $('freeze').disabled = true; $('export').disabled = true;
    } catch (error) { notice(error.message, 'error'); }
  }
}
function finishStroke() {
  if (!stroke) return;
  const current = stroke; stroke = null;
  if ($('calendar').hasPointerCapture(current.id)) $('calendar').releasePointerCapture(current.id);
  if (current.hand) return;
  if (state.drawing.end()) { persistDrawing(); generate(); } else updateEditor();
}
function changeDrawing(action) { finishStroke(); if (!canDraw()) return; if (state.drawing[action]()) { persistDrawing(); generate(); } }
function bindDrawing() {
  $('editor-modes').onclick = event => {
    const button = event.target.closest('[data-editor]'); if (!button || state.frozen || !state.ready) return;
    finishStroke(); state.editorMode = button.dataset.editor;
    if (state.editorMode === 'draw') state.mode = 'diff';
    persistDrawing(); updateEditor(); generate();
  };
  $('brushes').onclick = event => { const button = event.target.closest('[data-brush]'); if (!button || state.frozen) return; finishStroke(); state.brush = Number(button.dataset.brush); state.tool = 'brush'; persistDrawing(); updateEditor(); };
  $('draw-hand').onclick = () => { finishStroke(); state.tool = state.tool === 'hand' ? 'brush' : 'hand'; updateEditor(); };
  $('draw-undo').onclick = () => changeDrawing('undo'); $('draw-redo').onclick = () => changeDrawing('redo'); $('draw-clear').onclick = () => changeDrawing('clear');
  $('draw-zoom').onchange = () => { finishStroke(); const shell = document.querySelector('.graph-shell'); for (const n of [2, 3]) shell.classList.toggle('zoom-' + n, $('draw-zoom').value === String(n)); };
  $('calendar').addEventListener('pointerdown', event => {
    if (!canDraw() || (event.pointerType === 'mouse' && event.button !== 0) || !event.isPrimary) return;
    const point = pointerCell(event); if (!point) return;
    event.preventDefault(); finishStroke();
    const hand = state.tool === 'hand';
    stroke = { id: event.pointerId, last: point, hand, x: event.clientX, scroll: document.querySelector('.graph-shell').scrollLeft };
    $('calendar').setPointerCapture(event.pointerId);
    if (!hand) { state.drawing.begin(); paintCells([point]); }
  });
  $('calendar').addEventListener('pointermove', event => {
    if (!stroke || stroke.id !== event.pointerId) return;
    if (!canDraw()) { finishStroke(); return; }
    if (stroke.hand) { document.querySelector('.graph-shell').scrollLeft = stroke.scroll + stroke.x - event.clientX; return; }
    event.preventDefault(); const point = pointerCell(event);
    if (!point) { stroke.last = null; return; }
    paintCells(stroke.last ? gridLine(stroke.last, point) : [point]); stroke.last = point;
  });
  for (const type of ['pointerup', 'pointercancel', 'lostpointercapture']) $('calendar').addEventListener(type, event => { if (stroke?.id === event.pointerId) finishStroke(); });
  window.addEventListener('blur', finishStroke);
  document.addEventListener('keydown', event => {
    if (!canDraw() || /^(INPUT|SELECT|TEXTAREA)$/.test(event.target.tagName) || event.target.isContentEditable || document.querySelector('dialog[open]')) return;
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z') { event.preventDefault(); changeDrawing(event.shiftKey ? 'redo' : 'undo'); }
  });
}
function showDay(index) {
  const p = plan(), d = p?.days[Number(index)]; if (!d) return;
  $('day-detail').hidden = false;
  $('day-detail').innerHTML = `<strong>${esc(d.date)}</strong>Уже есть: ${p.existing.known ? d.existing : 'неизвестно'} · Новых: ${d.new} · Итого: ${d.projected}<br>${d.timestamps.map(t => `<time>${esc(t.slice(11, 19))}</time>`).join('')}${d.timestamps.length ? ' · ' + esc(p.config.timezone) : ''}`;
}
function setFrozen(value) {
  finishStroke(); state.drawing.locked = !!value;
  state.frozen = value; state.ticket = value?.ticket || null;
  if (value?.ticket?.repository) $('repository').value = value.ticket.repository.split('/')[1];
  $('repository').readOnly = !!value?.attempted;
  $('receipt').hidden = true;
  document.querySelectorAll('#settings input, #settings select, [data-profile], #reset, #regenerate').forEach(x => x.disabled = !!value);
  $('freeze').hidden = !!value; $('publication').hidden = false;
  $('publish-title').textContent = value ? 'Рисунок зафиксирован' : 'Выбери обновлённый рисунок';
  $('publish-description').textContent = value ? 'Проверь доступ и репозиторий. Отправим именно этот план.' : 'После изменения текущей активности план нужно выбрать заново.';
  if (value) store.set(key('frozen'), value); else store.remove(key('frozen'));
  render();
}
async function refreshBaseline() {
  const epoch = ++state.epoch;
  if (state.frozen) setFrozen(null);
  if (!$('use-existing').checked || !state.account) { state.snapshot = { known: false }; state.baselineLoading = false; $('existing-status').textContent = 'Текущая активность не учитывается.'; generate(); return; }
  state.baselineLoading = true; state.snapshot = { known: false }; render();
  $('existing-status').textContent = 'Загружаю календарь GitHub…';
  try {
    const snapshot = await api(`/api/calendar?start=${encodeURIComponent(state.config.start)}&end=${encodeURIComponent(state.config.end)}`);
    if (epoch !== state.epoch) return;
    state.snapshot = snapshot; store.set(key('snapshot'), snapshot);
    $('existing-status').textContent = 'Календарь обновлён. Посмотри результат и зафиксируй его ещё раз.';
  } catch (error) { if (epoch !== state.epoch) return; $('existing-status').textContent = error.message; notice(error.message, 'error'); }
  if (epoch !== state.epoch) return;
  state.baselineLoading = false; generate();
}
async function loadAccount() {
  if (!session()) return;
  $('auth-status').textContent = 'Проверяю подключение…';
  try {
    const account = await api('/api/me'); state.account = account; state.authExpired = false;
    state.config.author = account.author;
    $('account-label').textContent = '@' + account.login; $('repository-owner').textContent = account.login + ' /';
    $('connect').hidden = true; $('refresh-account').hidden = false; $('revoke').hidden = false;
    $('installation-help').hidden = account.installation.installed; $('install-app').href = account.installUrl;
    $('auth-status').textContent = account.installation.installed ? 'GitHub подключён. Авторизация действует не более часа.' : 'Вход выполнен. Теперь разреши приложению создать отдельный репозиторий.';
    if (account.installation.selection === 'all') $('auth-status').textContent += ' При установке выбран доступ ко всем репозиториям; его можно сузить в настройках GitHub.';
    const saved = store.get(key('frozen'));
    if (saved?.plan?.config.author.email === account.author.email && !saved.receipt) {
      restoreFrozen(saved);
      $('existing-status').textContent = 'Восстановлен зафиксированный план. При повторной отправке используется та же история.';
    } else await refreshBaseline();
    $('publication').hidden = false;
    const receipt = store.get(key('receipt')); if (matchingReceipt(receipt, state.frozen)) showReceipt(receipt);
  } catch (error) { $('auth-status').textContent = error.message; notice(error.message, 'error'); }
}
function exportPlan() {
  if (!flushPlan()) return;
  const p = plan(); if (!p) return;
  const format = $('export-format').value, svg = graphSVG(p, true), filename = 'activity-forge-' + p.id.slice(0, 12);
  if (format === 'json') return download(JSON.stringify(p, null, 2), filename + '.json', 'application/json');
  if (format === 'svg') return download(svg, filename + '.svg', 'image/svg+xml');
  if (format === 'csv') return download('date,existing,new,projected\n' + p.days.map(d => `${d.date},${p.existing.known ? d.existing : ''},${d.new},${d.projected}`).join('\n'), filename + '.csv', 'text/csv;charset=utf-8');
  if (format === 'html') return download(`<!doctype html><html lang="ru"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Activity Forge — выбранный план</title><body style="background:#121719;color:#c9d8cf;font-family:system-ui;padding:30px"><h1>Activity Forge</h1><p>${esc(p.config.start)} — ${esc(p.config.end)} · ${p.statistics.planned} новых коммитов</p>${svg}<p>План: ${p.id}</p><p>Декоративная история. Оттенки GitHub могут отличаться.</p></body></html>`, filename + '.html', 'text/html');
  const image = new Image(), url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
  image.onload = () => { const canvas = document.createElement('canvas'); canvas.width = image.width; canvas.height = image.height; canvas.getContext('2d').drawImage(image, 0, 0); URL.revokeObjectURL(url); canvas.toBlob(blob => { if (blob) download(blob, filename + '.png', 'image/png'); }); };
  image.onerror = () => { URL.revokeObjectURL(url); notice('Не получилось сохранить PNG. Выбери SVG.', 'error'); }; image.src = url;
}
function download(value, filename, type) { const url = URL.createObjectURL(value instanceof Blob ? value : new Blob([value], { type })); const a = document.createElement('a'); a.href = url; a.download = filename; a.click(); setTimeout(() => URL.revokeObjectURL(url), 30000); }
function showReceipt(receipt) {
  $('receipt').hidden = false; $('publication').hidden = true; $('freeze').hidden = true;
  $('publish-title').textContent = 'История опубликована'; $('publish-description').textContent = 'GitHub подтвердил точный HEAD выбранного плана.';
  const target = new URL(receipt.url); if (target.origin !== 'https://github.com') return;
  $('receipt').innerHTML = `<p><a class="button secondary" href="${esc(target.href)}" target="_blank" rel="noreferrer">Открыть ${esc(receipt.repository)} ↗</a></p><p>${num(receipt.commits)} коммитов · приватный репозиторий · ${esc(receipt.branch)}</p><code>${esc(receipt.head)}</code><p class="small muted">На странице профиля GitHub открой Contribution settings и включи Private contributions. Календарь может обновиться с задержкой.</p><p class="small muted">Теперь можно отозвать доступ GitHub кнопкой внизу страницы. Удаление установки приложения доступно в настройках GitHub.</p>`;
}
function startLogin() {
  if (!configReady) return notice('Подключение GitHub ещё настраивается. Превью и экспорт уже доступны.');
  const nonce = Array.from(crypto.getRandomValues(new Uint8Array(32)), b => b.toString(16).padStart(2, '0')).join('');
  try { sessionStorage.setItem('af.oauth-nonce', nonce); } catch { return notice('Для безопасного входа разреши хранилище этой вкладки.', 'error'); }
  location.assign(API_ORIGIN + '/oauth/start?nonce=' + nonce);
}
async function prepare() {
  if (!state.frozen) return;
  $('prepare').disabled = true;
  try {
    const f = state.frozen;
    const ticket = await api('/api/prepare', { payload: f.payload, planId: f.plan.id, repository: publicationDestination(f, $('repository').value) });
    state.ticket = ticket; f.ticket = ticket; store.set(key('frozen'), f);
    notice('');
    $('confirm-details').innerHTML = [['Репозиторий', ticket.repository], ['Доступ', 'Приватный'], ['Основная ветка', 'Ветка по умолчанию в новом репозитории'], ['Новых коммитов', num(ticket.commits)], ['Период', f.plan.config.start + ' — ' + f.plan.config.end], ['План', ticket.planId.slice(0, 16) + '…']].map(([a, b]) => `<div><dt>${esc(a)}</dt><dd>${esc(b)}</dd></div>`).join('');
    $('agree').checked = false; $('approve').disabled = true; $('publish-progress').textContent = ''; $('confirm-dialog').showModal();
  } catch (error) { notice(error.message, 'error'); }
  render();
}
async function publishSelected() {
  if (!$('agree').checked || !state.ticket || state.publishing) return;
  state.publishing = true; $('approve').disabled = true; $('cancel-publish').disabled = true;
  state.frozen.attempted = true; state.frozen.ticket = state.ticket; store.set(key('frozen'), state.frozen); $('repository').readOnly = true;
  $('publish-progress').textContent = 'Собираю историю и отправляю в GitHub. Не закрывай вкладку. После ответа будет проверен точный HEAD.';
  try {
    const receipt = await api('/api/publish', { ticket: state.ticket.ticket, payload: state.frozen.payload, planId: state.frozen.plan.id, confirmed: true });
    store.set(key('receipt'), receipt); $('confirm-dialog').close(); showReceipt(receipt); notice('GitHub подтвердил публикацию ' + num(receipt.commits) + ' коммитов.', 'success');
  } catch (error) { $('publish-progress').textContent = error.message + ' Не выбирай новый план: повторная отправка этого плана проверит результат и не добавит дубликаты.'; $('approve').textContent = 'Проверить и повторить'; $('approve').disabled = false; }
  state.publishing = false; $('cancel-publish').disabled = false;
}
function bind() {
  bindDrawing();
  $('settings').addEventListener('submit', e => e.preventDefault());
  $('settings').addEventListener('input', event => {
    if (event.target.id === 'intensity') state.config.mean = [0, .55, .95, 1.6, 2.3, 3.2, 4.5, 5.9, 7.4, 9.2, 11][Number(event.target.value)];
    if (['start', 'end'].includes(event.target.id)) $('period').value = 'custom';
    readControls(); schedule();
  });
  $('settings').addEventListener('change', event => { if (['start', 'end', 'period'].includes(event.target.id) && state.account) refreshBaseline(); });
  $('period').addEventListener('change', () => {
    if ($('period').value === 'custom') return;
    const end = new Date(); end.setHours(12, 0, 0, 0); const start = new Date(end); start.setDate(start.getDate() - Number($('period').value) + 1);
    const date = d => [d.getFullYear(), String(d.getMonth() + 1).padStart(2, '0'), String(d.getDate()).padStart(2, '0')].join('-');
    state.config.start = date(start); state.config.end = date(end); setControls(); generate();
  });
  $('profiles').addEventListener('click', event => {
    const button = event.target.closest('[data-profile]'); if (!button || state.frozen || state.editorMode === 'draw') return;
    const previous = state.config; state.profile = button.dataset.profile;
    state.config = generator({ action: 'defaults', profile: state.profile }).config;
    for (const field of ['start', 'end', 'timezone', 'seed', 'author']) state.config[field] = previous[field];
    setControls(); generate();
  });
  $('regenerate').onclick = () => { state.config.seed = crypto.getRandomValues(new Uint32Array(1))[0] & 0x7fffffff; setControls(); generate(); };
  $('reset').onclick = () => { state.profile = 'bursts'; state.config = generator({ action: 'defaults', profile: state.profile }).config; setControls(); state.account ? refreshBaseline() : generate(); };
  $('modes').onclick = event => { const button = event.target.closest('[data-mode]'); if (!button) return; finishStroke(); state.mode = button.dataset.mode; render(); };
  $('calendar').onclick = event => { if (state.editorMode === 'draw' && !state.frozen && state.mode !== 'current') return; const cell = event.target.closest('[data-day]'); if (cell) showDay(cell.dataset.day); };
  $('calendar').onkeydown = event => {
    if (event.target.dataset.day === undefined) return;
    const index = Number(event.target.dataset.day);
    const shift = { ArrowRight: 7, ArrowLeft: -7, ArrowDown: 1, ArrowUp: -1 }[event.key];
    if (shift) { event.preventDefault(); $('calendar').querySelector(`[data-day="${index + shift}"]`)?.focus({ preventScroll: false }); return; }
    if (!['Enter', ' '].includes(event.key)) return;
    event.preventDefault();
    if (!canDraw() || state.tool === 'hand') return showDay(index);
    const layout = calendarLayout(plan().config.start, plan().days.length), n = index + layout.offset;
    state.drawing.begin(); paintCells([{ x: Math.floor(n / 7), y: n % 7 }]); state.drawing.end(); persistDrawing(); generate();
    $('calendar').querySelector(`[data-day="${index}"]`)?.focus({ preventScroll: true });
  };
  $('freeze').onclick = () => { if (!flushPlan() || !state.result?.plan.statistics.planned || state.baselineLoading) return; const f = structuredClone(state.result); f.editor = editorSnapshot(); setFrozen(f); $('publish-panel').scrollIntoView({ behavior: 'smooth', block: 'start' }); };
  $('edit').onclick = () => { setFrozen(null); generate(); };
  $('connect').onclick = startLogin; $('refresh-account').onclick = loadAccount; $('use-existing').onchange = refreshBaseline;
  $('prepare').onclick = prepare; $('agree').onchange = () => $('approve').disabled = !$('agree').checked;
  $('approve').onclick = publishSelected; $('cancel-publish').onclick = () => $('confirm-dialog').close();
  $('confirm-dialog').addEventListener('cancel', e => { if (state.publishing) e.preventDefault(); });
  $('export').onclick = exportPlan;
  for (const id of ['privacy-open', 'privacy-more']) $(id).onclick = () => $('privacy-dialog').showModal();
  $('privacy-close').onclick = () => $('privacy-dialog').close();
  $('theme').onclick = () => { document.documentElement.dataset.theme = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark'; store.set('theme', document.documentElement.dataset.theme); render(); };
  $('clear').onclick = () => { if (state.publishing) return; if (!confirm('Удалить настройки, планы и авторизацию из этого браузера? Доступ приложения в GitHub останется, пока ты не отзовёшь его отдельно.')) return; for (let i = localStorage.length - 1; i >= 0; i--) { const k = localStorage.key(i); if (k.startsWith('af.')) localStorage.removeItem(k); } sessionStorage.removeItem('af.session'); sessionStorage.removeItem('af.oauth-nonce'); location.reload(); };
  $('revoke').onclick = async () => { if (state.publishing) return; try { await api('/api/revoke', {}); sessionStorage.removeItem('af.session'); $('revoke').hidden = true; $('connect').hidden = false; $('refresh-account').hidden = true; state.account = null; $('prepare').disabled = true; notice('GitHub подтвердил отзыв авторизации. Установку приложения можно удалить отдельно в настройках GitHub.', 'success'); } catch (error) { notice(error.message + ' Настройки: github.com/settings/apps/authorizations', 'error'); } };
}
async function init() {
  document.documentElement.dataset.theme = store.get('theme', 'dark');
  bind();
  try {
    const go = new Go();
    const response = await fetch('./generator.wasm', { cache: 'default' });
    if (!response.ok) throw new Error('Не удалось загрузить генератор. Обнови страницу.');
    const result = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject);
    go.run(result.instance).catch(() => notice('Генератор остановился. Сохранённые планы остались в браузере.', 'error'));
    state.ready = true;
    const defaults = generator({ action: 'defaults', profile: 'bursts' }).config;
    const saved = store.get('settings'); state.config = saved?.config || defaults; state.profile = saved?.profile || 'bursts';
    delete state.config.manual;
    const drawing = store.get('drawing'); state.drawing = new Drawing(drawing?.cells); state.editorMode = drawing?.mode === 'draw' ? 'draw' : 'generate';
    if (BRUSH_COUNTS.includes(drawing?.brush)) state.brush = drawing.brush;
    if (state.editorMode === 'draw') state.mode = 'diff';
    state.config.author = { name: '', email: '' };
    if (state.config.end > defaults.end || !state.config.start) state.config = defaults;
    $('start').max = defaults.end; $('end').max = defaults.end;
    setControls(); $('regenerate').disabled = false; generate();
    const cached = store.get('frozen.anonymous'); if (cached?.payload && !session()) restoreFrozen(cached);
    await loadAccount();
  } catch (error) { notice(error.message + ' Превью требует современного браузера с WebAssembly.', 'error'); }
  try { const result = await api('/api/config'); configReady = result.configured; if (!configReady) $('auth-status').textContent = 'Подключение GitHub ещё настраивается. Превью и экспорт доступны.'; }
  catch { $('auth-status').textContent = 'Сервер GitHub временно недоступен. Превью и экспорт работают локально.'; }
}
init();
