const paths = {
  down: '<path d="m4.5 6.5 3.5 3.5 3.5-3.5"/>',
  right: '<path d="m6.5 4.5 3.5 3.5-3.5 3.5"/>',
  close: '<path d="m5 5 6 6m0-6-6 6"/>',
  pin: '<path d="m9 2 5 5-2 1-2 3-2-2-4 4M5 6l3-2 1-2M5 6l-2 1 6 6 1-2"/>',
  refresh: '<path d="M13 6a5.2 5.2 0 1 0 0 4M13 2v4H9"/>',
  clock: '<circle cx="8" cy="8" r="5.25"/><path d="M8 4.8V8l2.2 1.4"/>',
  check: '<path d="m3.5 8 3 3 6-6"/>',
  compare: '<rect x="2" y="3" width="5" height="10" rx="1.2"/><rect x="9" y="3" width="5" height="10" rx="1.2"/>',
  back: '<path d="m7 4-4 4 4 4M3 8h10"/>',
  empty: '<rect x="2.5" y="3" width="11" height="10" rx="2"/><path d="M6 8h4M8 6v4"/>',
  alert: '<path d="m8 2 6 11H2L8 2Z"/><path d="M8 6v3M8 11h.01"/>',
};
const icon = (name, extra = '') => `<svg class="icon ${extra}" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.45" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name]}</svg>`;
const agents = [
  { id: 'claude', name: 'Claude', plan: 'Pro', short: 19, long: 70, window: '5h', reset: '12m', clock: '16:38', longReset: '16 Oct, 12:00' },
  { id: 'kiro', name: 'Kiro', plan: 'Free', short: 3, window: 'month', reset: '1 Nov', note: '1.66 of 50 credits' },
  { id: 'codex', name: 'Codex', plan: '', short: 0, long: 14, window: '5h' },
  { id: 'cursor', name: 'Cursor', plan: 'Pro', short: 66, window: 'plan', reset: '28 Oct' },
  { id: 'copilot', name: 'Copilot', plan: '', short: 13, window: 'premium', reset: '1 Nov' },
  { id: 'devin', name: 'Devin', plan: 'Pro', short: 0, long: 38, window: 'day', reset: '16:00' },
  { id: 'antigravity', name: 'Antigravity', plan: '', short: 0, long: 10, window: '5h', reset: '4h 59m', clock: '21:25', longReset: '19 Oct, 17:48' },
];
const concepts = {
  A: { name: 'Orbit', phrase: 'Everything at a glance.', description: 'Seven familiar marks, a quiet rail, and just enough detail. A considered evolution of your preferred direction.', advantage: 'All seven agents stay visible. The inspector reads in one pass.', trade: 'The full rail takes more horizontal space.', signature: 'Seven rings. One glance.', width: 386 },
  B: { name: 'Ledger', phrase: 'A clearer comparison.', description: 'A compact instrument for comparing short and long limits. Every number and reset has a place.', advantage: 'Aligned windows make differences easy to spot.', trade: 'The inspector is wider and contains more data.', signature: 'A precise, miniature instrument.', width: 412 },
  C: { name: 'Focus', phrase: 'Only what you need.', description: 'A pocket-sized tab opens into a focused view. Choose an agent, then see its quota windows without competing detail.', advantage: 'The smallest resting presence and the clearest individual breakdown.', trade: 'Comparing windows takes an extra selection.', signature: 'A small presence. Room for detail.', width: 434 },
};
const saved = (key, fallback = '') => {
  try { return localStorage.getItem(key) || fallback; } catch { return fallback; }
};
const persist = (key, value) => {
  try { localStorage.setItem(key, value); } catch { /* The prototype remains usable when storage is unavailable. */ }
};
const initial = new URLSearchParams(location.search).get('concept');
const state = {
  concept: concepts[initial] ? initial : 'A', compare: initial === 'compare',
  scene: 'normal', desktop: 'dark', compact: false, selected: 'claude',
  pinned: { A: false, B: false, C: false }, open: { A: true, B: true, C: true },
  clocks: new Set(), groups: false, recovery: false, refreshing: false, refreshed: false,
  preferred: saved('agentquota-refinement-choice', 'A'), notes: saved('agentquota-refinement-notes'),
  status: '', reveal: '',
};
const tone = value => value >= 90 ? 'danger' : value >= 70 ? 'warning' : 'healthy';
const readingState = agent => state.scene === 'loading' ? 'loading' : state.scene === 'issues' && agent.id === 'claude' ? 'stale' : state.scene === 'issues' && agent.id === 'devin' ? 'failed' : 'ok';
const shortUsed = agent => state.scene === 'critical' && agent.id === 'claude' ? 71 : state.scene === 'critical' && agent.id === 'cursor' ? 91 : agent.short;
const longUsed = agent => state.scene === 'critical' && agent.id === 'claude' ? 94 : agent.long;
const used = agent => Math.max(shortUsed(agent), longUsed(agent) ?? 0);
const available = agent => !['loading', 'failed'].includes(readingState(agent));
const value = agent => available(agent) ? used(agent) : null;
const percentage = (number, extra = '') => `<span class="percent ${number === null ? 'unavailable' : tone(number)} ${extra}">${number === null ? '—' : number + '<small>%</small>'}</span>`;
const mark = (agent, extra = '') => `<span class="mark ${agent.id} ${extra}" aria-hidden="true"><img src="${MARKS[agent.id]}" alt="" draggable="false"></span>`;
const ring = agent => `<span class="ring ${readingState(agent)} ${tone(used(agent))}" style="--used:${value(agent) ?? 0}"><svg viewBox="0 0 30 30" aria-hidden="true"><circle class="ring-track" cx="15" cy="15" r="12.7"/><circle class="ring-fill" cx="15" cy="15" r="12.7" pathLength="100"/></svg>${mark(agent)}</span>`;
const track = number => `<span class="usage-track ${number === null ? '' : tone(number)}" style="--used:${number ?? 0}%"><span></span></span>`;
const resetText = agent => state.clocks.has(agent.id) && agent.clock ? agent.clock : agent.reset;
const toggleReset = (agent, extra = '') => {
  if (!agent.reset) return '<span class="unknown-reset" title="Reset not reported">—</span>';
  const text = resetText(agent);
  const label = agent.clock ? `${state.clocks.has(agent.id) ? 'At' : 'In'} ${text}` : text;
  if (!agent.clock) return `<span class="reset static-reset ${extra}">${icon('clock')}${label}</span>`;
  return `<button class="reset ${extra}" data-action="clock" data-id="${agent.id}" title="Switch this 5-hour reset between countdown and clock" aria-label="${agent.name}: 5-hour window resets ${label.toLowerCase()}; switch countdown and clock">${icon('clock')}<span class="reset-window">5h</span>${label}</button>`;
};
const groupTable = () => `<table class="group-table"><caption class="sr-only">Antigravity quota groups, percentage used</caption><thead><tr><th>Model group</th><th>5h</th><th>Week</th></tr></thead><tbody><tr><th>Gemini</th><td>0<small>%</small></td><td>2<small>%</small></td></tr><tr><th>Claude / GPT</th><td>0<small>%</small></td><td>10<small>%</small></td></tr></tbody></table>`;
const failure = () => `<div class="failure-message">Sign-in expired.<button class="recovery-button" data-action="recovery" aria-expanded="${state.recovery}">${state.recovery ? 'Hide help' : 'How to recover'}${icon(state.recovery ? 'down' : 'right')}</button></div>${state.recovery ? '<p class="recovery-help">Open Devin and sign in. Then return here and choose Refresh.</p>' : ''}`;
const stale = () => `<span class="stale-message">Update failed · last good 8 min ago</span>`;

function panelHead(variant) {
  return `<header class="panel-head"><h3>Usage limits</h3><span class="agent-count">${state.scene === 'empty' ? '0' : '7'} agents</span><div class="panel-actions"><button class="icon-button ${state.pinned[variant] ? 'is-pinned' : ''}" data-action="pin" data-v="${variant}" aria-label="${state.pinned[variant] ? 'Unpin' : 'Pin'} ${concepts[variant].name} panel" aria-pressed="${state.pinned[variant]}" title="${state.pinned[variant] ? 'Unpin panel' : 'Keep panel open'}">${icon('pin')}</button><button class="icon-button" data-action="close" data-v="${variant}" aria-label="Close ${concepts[variant].name} panel" title="Close panel">${icon('close')}</button></div></header>`;
}
function panelFoot(variant) {
  const label = state.refreshing || state.scene === 'loading' ? 'Checking usage…' : state.scene === 'issues' ? '1 stale · 1 unavailable' : state.refreshed ? 'Checked just now' : 'Updated just now';
  return `<footer class="panel-footer"><span class="freshness ${state.scene === 'issues' ? 'needs-attention' : ''}" role="status">${state.scene === 'issues' ? icon('alert') : '<span class="freshness-dot"></span>'}${label}</span><button class="refresh-button ${state.refreshing ? 'is-loading' : ''}" data-action="refresh" data-v="${variant}" ${state.refreshing || state.scene === 'loading' ? 'disabled' : ''}>${icon('refresh')}<span>${state.refreshing ? 'Checking' : 'Refresh'}</span></button></footer>`;
}
const emptyPanel = () => `<div class="empty-state">${icon('empty')}<h4>No agents enabled</h4><p>Right-click the widget to choose<br>which agents to show.</p></div>`;
function rail(variant, compact = false) {
  const selected = agents.find(agent => agent.id === state.selected);
  if (state.scene === 'empty') return `<button class="quota-rail empty-rail" data-action="open" data-v="${variant}" aria-label="Open ${concepts[variant].name} panel">${icon('empty')}<span>No agents enabled</span>${icon('down')}</button>`;
  if (compact) {
    const maximum = Math.max(...agents.filter(available).map(used), 0);
    return `<button class="quota-rail compact-rail ${variant === 'B' ? 'compact-ledger' : ''}" data-action="open" data-v="${variant}" aria-label="Open ${concepts[variant].name} panel" aria-expanded="${state.open[variant]}">${variant === 'B' ? `<span class="mini-meter ${tone(maximum)}"><i></i><i></i><i></i></span>${percentage(state.scene === 'loading' ? null : maximum)}<span class="max-label">max</span>` : `${ring(selected)}${percentage(value(selected))}`}${icon('down', state.open[variant] ? '' : 'rotate-closed')}</button>`;
  }
  return `<div class="quota-rail full-rail rail-${variant}" role="group" aria-label="${concepts[variant].name} all-agent rail">${agents.map(agent => {
    const number = value(agent);
    const content = variant === 'A' ? `${ring(agent)}<span class="rail-copy">${percentage(number)}${agent.clock && available(agent) ? `<span class="rail-reset">${resetText(agent).replaceAll(' ', '')}</span>` : ''}</span>` : variant === 'B' ? `<span class="rail-meter-label">${mark(agent)}${percentage(number)}</span>${track(number)}` : `${mark(agent)}${percentage(number)}`;
    return `<button class="rail-agent ${readingState(agent)} ${variant === 'C' && state.selected === agent.id ? 'is-selected' : ''}" data-action="agent" data-id="${agent.id}" data-v="${variant}" aria-label="${agent.name}: ${number === null ? readingState(agent) === 'loading' ? 'checking' : 'unavailable' : number + ' percent used'}">${content}</button>`;
  }).join('')}<span class="rail-divider"></span><button class="icon-button rail-open" data-action="open" data-v="${variant}" aria-label="${state.open[variant] ? 'Toggle' : 'Open'} ${concepts[variant].name} panel" aria-expanded="${state.open[variant]}">${icon('down', state.open[variant] ? '' : 'rotate-closed')}</button></div>`;
}
function orbit() {
  return `<div class="orbit-rows">${agents.map(agent => {
    const status = readingState(agent);
    let detail;
    if (status === 'loading') detail = '<span class="skeleton skeleton-detail"></span>';
    else if (status === 'failed') detail = failure();
    else if (agent.id === 'antigravity') detail = `<div class="row-detail"><button class="group-disclosure" data-action="groups" aria-expanded="${state.groups}">2 model groups${icon(state.groups ? 'down' : 'right')}</button>${toggleReset(agent)}</div>${state.groups ? groupTable() : ''}`;
    else detail = `<div class="row-detail"><span class="inline-metrics">${agent.long !== undefined ? `<span>${agent.window === 'day' ? 'Day' : '5h'} <b>${shortUsed(agent)}%</b></span><span>Week <b>${longUsed(agent)}%</b></span>` : `<span>${agent.id === 'kiro' ? agent.note : agent.id === 'copilot' ? 'Premium interactions' : 'Plan usage'}</span>`}</span>${toggleReset(agent)}</div>${status === 'stale' ? stale() : ''}`;
    return `<div class="orbit-row ${status}">${mark(agent)}<div class="orbit-row-content"><div class="row-top"><span class="agent-name">${agent.name}${agent.plan ? `<span class="plan-name">${agent.plan}</span>` : ''}</span>${percentage(value(agent))}</div>${detail}</div></div>`;
  }).join('')}</div>`;
}
function ledger() {
  return `<table class="ledger-table"><caption class="sr-only">Quota windows and resets, percentage used</caption><thead><tr><th>Agent</th><th>Short</th><th>Week</th><th>Reset</th></tr></thead><tbody>${agents.map(agent => {
    const status = readingState(agent);
    const short = available(agent) ? shortUsed(agent) : null;
    const long = available(agent) && agent.long !== undefined ? longUsed(agent) : null;
    return `<tr class="${status}"><th scope="row"><span class="ledger-identity">${mark(agent)}<span>${agent.name}<small>${agent.plan || (agent.id === 'antigravity' ? '2 groups' : 'Usage')}</small></span></span></th><td>${percentage(short)}<span class="window-caption">${agent.window}</span>${track(short)}</td><td>${percentage(long)}<span class="window-caption">${agent.long === undefined ? '—' : 'week'}</span>${agent.long === undefined ? '<span class="track-spacer"></span>' : track(long)}</td><td class="ledger-reset">${status === 'loading' || status === 'failed' ? '<span class="unknown-reset">—</span>' : toggleReset(agent)}</td></tr>${status === 'stale' || status === 'failed' ? `<tr class="ledger-state"><td colspan="4">${status === 'stale' ? stale() : failure()}</td></tr>` : ''}`;
  }).join('')}</tbody></table><div class="ledger-disclosure"><button class="group-disclosure" data-action="groups" aria-expanded="${state.groups}">Antigravity model groups${icon(state.groups ? 'down' : 'right')}</button>${state.groups && state.scene !== 'loading' ? groupTable() : ''}</div>`;
}
function windowBlock(agent, label, number, long = false) {
  return `<section class="window-block"><div class="window-heading"><h5>${label}</h5>${percentage(number)}</div>${track(number)}<div class="window-reset">${long ? agent.longReset ? `${icon('clock')}Resets ${agent.longReset}` : 'Reset not reported' : agent.reset ? toggleReset(agent) : 'Reset not reported'}</div></section>`;
}
function focus() {
  const agent = agents.find(item => item.id === state.selected);
  const status = readingState(agent);
  const label = agent.window === '5h' ? '5-hour window' : agent.window === 'day' ? 'Daily window' : agent.window === 'month' ? 'Monthly credits' : agent.window === 'premium' ? 'Premium interactions' : 'Plan usage';
  const detail = status === 'loading' ? '<div class="focus-skeleton"><span class="skeleton"></span><span class="skeleton"></span><p>Checking usage…</p></div>' : status === 'failed' ? `<div class="focus-failure">${icon('alert')}<h5>Quota unavailable</h5>${failure()}</div>` : `${status === 'stale' ? stale() : ''}${agent.id === 'antigravity' ? `<h5 class="model-group-title">Gemini</h5>${windowBlock(agent, '5-hour window', 0)}${windowBlock(agent, 'Weekly window', 2, true)}<h5 class="model-group-title">Claude / GPT</h5>${windowBlock(agent, '5-hour window', 0)}${windowBlock(agent, 'Weekly window', 10, true)}` : `${windowBlock(agent, label, shortUsed(agent))}${agent.long === undefined ? '' : windowBlock(agent, 'Weekly window', longUsed(agent), true)}${agent.note ? `<p class="focus-note">${agent.note} used.</p>` : ''}`}`;
  return `<div class="focus-body"><nav class="agent-index" aria-label="Choose an agent in Focus">${agents.map(item => `<button class="index-agent ${item.id === state.selected ? 'is-selected' : ''} ${readingState(item)}" data-action="select" data-id="${item.id}" aria-pressed="${item.id === state.selected}">${mark(item)}<span>${item.name}</span>${percentage(value(item))}</button>`).join('')}</nav><div class="focus-detail"><div class="focus-identity">${mark(agent)}<div><h4>${agent.name}</h4><span>${agent.plan || 'Usage breakdown'}</span></div></div>${detail}</div></div>`;
}
function panel(variant) {
  if (!state.open[variant]) return `<div class="closed-hint">Click the ${state.compact || state.compare ? 'tab' : 'rail'} to reopen.</div>`;
  return `<section class="usage-panel panel-${variant} ${state.reveal === variant ? 'is-opening' : ''}" aria-label="${concepts[variant].name} usage panel">${panelHead(variant)}${state.scene === 'empty' ? emptyPanel() : variant === 'A' ? orbit() : variant === 'B' ? ledger() : focus()}${panelFoot(variant)}</section>`;
}
function widget(variant, compare = false) {
  return `<div class="widget-unit widget-${variant}" data-widget="${variant}"><div class="rail-anchor">${rail(variant, compare || state.compact)}</div><div class="inspector-anchor">${panel(variant)}</div></div>`;
}
function conceptTabs() {
  return `<div class="concept-tabs" role="group" aria-label="Design direction">${Object.keys(concepts).map(variant => `<button class="${state.concept === variant && !state.compare ? 'is-active' : ''}" data-action="concept" data-v="${variant}" aria-pressed="${state.concept === variant && !state.compare}"><span class="concept-letter">${variant}</span>${concepts[variant].name}${variant === 'A' ? '<span class="preferred-dot" title="Your preferred direction"></span>' : ''}</button>`).join('')}</div><button class="compare-button ${state.compare ? 'is-active' : ''}" data-action="compare" aria-pressed="${state.compare}">${icon('compare')}Side by side</button>`;
}
function controls() {
  return `<div class="preview-controls"><label for="scene">Preview state</label><select id="scene"><option value="normal">Normal usage</option><option value="critical">Near the limit</option><option value="issues">Stale & failed</option><option value="loading">Checking usage</option><option value="empty">No agents enabled</option></select><span class="control-divider"></span><label for="desktop">Desktop</label><select id="desktop"><option value="dark">Dark</option><option value="light">Light</option></select>${state.compare ? '' : `<div class="size-control" role="group" aria-label="Widget footprint"><button class="${!state.compact ? 'is-active' : ''}" data-action="footprint" data-value="full" aria-pressed="${!state.compact}">All agents</button><button class="${state.compact ? 'is-active' : ''}" data-action="footprint" data-value="compact" aria-pressed="${state.compact}">Compact</button></div>`}</div>`;
}
function studio() {
  const concept = concepts[state.concept];
  return `<section class="studio" aria-label="Inspect ${concept.name}"><div class="design-caption"><div><h2>${concept.phrase}</h2><p>${concept.description}</p></div><span class="actual-size">Actual size · ${concept.width} px inspector</span></div><div class="desktop desktop-${state.desktop}"><div class="desktop-note"><span class="desktop-dots"><i></i><i></i><i></i></span><span>Your workspace, uninterrupted.</span></div><div class="studio-widget">${widget(state.concept)}</div><div class="desktop-bottom"><span>Hover to inspect · click to pin · drag in the native app</span><span>Illustrative values · resets frozen</span></div></div><div class="design-rationale"><div><span class="rationale-label">Why it works</span><p>${concept.advantage}</p></div><div><span class="rationale-label">The trade-off</span><p>${concept.trade}</p></div><button class="keep-button" data-action="choose" data-v="${state.concept}">${state.preferred === state.concept ? icon('check') : ''}${state.preferred === state.concept ? concept.name + ' kept as the working direction' : 'Keep ' + concept.name}</button></div></section>`;
}
function comparison() {
  return `<section class="comparison" aria-label="Three design directions at actual size">${Object.keys(concepts).map(variant => `<article class="comparison-concept"><header><h2><span>${variant}</span>${concepts[variant].name}</h2>${variant === 'A' ? '<span class="your-pick">Your preferred base</span>' : ''}<p>${concepts[variant].signature}</p></header><div class="comparison-desktop desktop-${state.desktop}">${widget(variant, true)}</div><p class="compare-reason">${concepts[variant].advantage}</p><button class="review-button" data-action="concept" data-v="${variant}">Inspect ${concepts[variant].name}${icon('right')}</button></article>`).join('')}</section>`;
}
function render(restoreFocus = true) {
  const old = document.activeElement;
  const identity = restoreFocus && old && old !== document.body ? { id: old.id, action: old.dataset.action, agent: old.dataset.id, variant: old.dataset.v, footprint: old.dataset.value } : null;
  document.getElementById('app').innerHTML = `<div class="study-shell"><header class="study-header"><a class="product-name" href="/">Agent Quota<span class="version-note">Design study</span></a><a class="original-link" href="http://127.0.0.1:4187/" target="_blank" rel="noopener">Original concepts${icon('right')}</a></header><div class="study-title"><div><h1>Small presence.<br>Clear picture.</h1><p>A quieter companion for the way you work.</p></div><div class="study-intent"><span>A / Orbit is your preferred base.</span><p>Three refinements. One familiar purpose.<br>Every agent, a little easier to read.</p></div></div><nav class="study-navigation" aria-label="Compare concepts">${conceptTabs()}</nav>${controls()}${state.compare ? comparison() : studio()}<section class="feedback-section" aria-labelledby="feedback-title"><div><h2 id="feedback-title">What should we keep?</h2><p>Orbit is the foundation. Note the details you like<br>or the parts you would combine.</p><span class="local-note">Notes stay in this browser.</span></div><div class="feedback-input"><label class="sr-only" for="design-notes">Design feedback</label><textarea id="design-notes" rows="3" placeholder="The rail from Orbit, the compact tab from Focus…"></textarea><div><span>Working direction: <b>${concepts[state.preferred]?.name || 'Orbit'}</b></span><button class="save-notes" data-action="save-notes">Save notes${icon('check')}</button></div></div></section><footer class="study-footer"><p>Interactive design prototypes · sample data · no live account reads</p><p>Inter · existing agent marks · original quota thresholds</p></footer><div class="announcement" role="status" aria-live="polite">${state.status}</div></div>`;
  document.getElementById('scene').value = state.scene;
  document.getElementById('desktop').value = state.desktop;
  document.getElementById('design-notes').value = state.notes;
  if (identity) {
    const candidates = identity.id ? [document.getElementById(identity.id)] : [...document.querySelectorAll('[data-action]')];
    const next = candidates.find(element => element && (identity.id || element.dataset.action === identity.action && element.dataset.id === identity.agent && element.dataset.v === identity.variant && element.dataset.value === identity.footprint));
    next?.focus({ preventScroll: true });
  }
}
function updateURL() {
  const url = new URL(location.href);
  url.searchParams.set('concept', state.compare ? 'compare' : state.concept);
  history.replaceState(null, '', url);
}
let hoverTimer;
let leaveTimer;
let refreshTimer;
document.addEventListener('click', event => {
  const control = event.target.closest('[data-action]');
  if (!control || control.disabled) return;
  const { action, v: variant, id } = control.dataset;
  state.status = '';
  state.reveal = '';
  clearTimeout(hoverTimer);
  clearTimeout(leaveTimer);
  switch (action) {
    case 'concept': state.concept = variant; state.compare = false; state.open[variant] = true; state.reveal = variant; updateURL(); break;
    case 'compare': state.compare = !state.compare; updateURL(); break;
    case 'footprint': state.compact = control.dataset.value === 'compact'; break;
    case 'agent': state.selected = id; state.open[variant] = true; state.pinned[variant] = true; break;
    case 'select': state.selected = id; break;
    case 'open': state.pinned[variant] = !state.pinned[variant] || !state.open[variant]; state.open[variant] = state.pinned[variant]; if (state.open[variant]) state.reveal = variant; break;
    case 'pin': state.pinned[variant] = !state.pinned[variant]; break;
    case 'close': state.open[variant] = false; state.pinned[variant] = false; break;
    case 'clock': state.clocks.has(id) ? state.clocks.delete(id) : state.clocks.add(id); break;
    case 'groups': state.groups = !state.groups; break;
    case 'recovery': state.recovery = !state.recovery; break;
    case 'choose': state.preferred = variant; persist('agentquota-refinement-choice', variant); state.status = `${concepts[variant].name} saved locally as the working direction.`; break;
    case 'save-notes': persist('agentquota-refinement-notes', state.notes); state.status = 'Design notes saved in this browser.'; break;
    case 'refresh':
      if (state.refreshing) return;
      state.refreshing = true;
      clearTimeout(refreshTimer);
      refreshTimer = setTimeout(() => {
        state.refreshing = false;
        state.refreshed = true;
        state.status = state.scene === 'issues' ? 'Preview refreshed. Stale and failed readings remain visible.' : 'Preview refreshed using illustrative data.';
        render();
      }, 850);
      break;
    default: return;
  }
  render();
});
document.addEventListener('change', event => {
  if (event.target.id === 'scene') {
    clearTimeout(refreshTimer);
    state.scene = event.target.value;
    state.refreshing = false;
    state.refreshed = false;
    state.reveal = '';
    state.status = '';
    for (const variant of Object.keys(concepts)) state.open[variant] = true;
    render();
  } else if (event.target.id === 'desktop') {
    state.desktop = event.target.value;
    render();
  }
});
document.addEventListener('input', event => {
  if (event.target.id === 'design-notes') state.notes = event.target.value;
});
document.addEventListener('pointerover', event => {
  const unit = event.target.closest('[data-widget]');
  if (!unit || state.compare || unit.contains(event.relatedTarget)) return;
  clearTimeout(leaveTimer);
  const variant = unit.dataset.widget;
  if (!state.open[variant]) hoverTimer = setTimeout(() => { state.open[variant] = true; state.reveal = variant; render(false); }, 400);
});
document.addEventListener('pointerout', event => {
  const unit = event.target.closest('[data-widget]');
  if (!unit || state.compare || unit.contains(event.relatedTarget)) return;
  clearTimeout(hoverTimer);
  const variant = unit.dataset.widget;
  if (!state.pinned[variant]) leaveTimer = setTimeout(() => { state.open[variant] = false; render(false); }, 300);
});
render(false);
