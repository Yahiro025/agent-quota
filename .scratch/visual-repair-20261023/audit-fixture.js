// Mocked-fixture audit: drives the Bloom quota space on Vite with tauri-mock.
const { chromium } = require('C:/Users/Bennett/Documents/PROJECT/agent-quota/.scratch/qa/node_modules/playwright-core');
const OUT = 'C:/Users/Bennett/Documents/PROJECT/agent-quota/.scratch/visual-repair-20261023';
const later = (ms) => new Date(Date.now() + ms).toISOString();
const H = 3600e3;

const SEVEN = [
  { id: 'claude', state: 'failed', err: 'Sign-in expired.', help: 'Open Claude Code to renew its sign-in, then choose Refresh.' },
  { id: 'kiro', state: 'ok', used: 62, detail: 'Credits', windows: [{ label: 'credits', used: 62 }] },
  { id: 'codex', state: 'ok', used: 10, windows: [{ label: '5h', used: 10 }], reset5h: later(3.4 * H) },
  { id: 'cursor', state: 'ok', used: 71 },
  { id: 'copilot', state: 'stale', used: 90, err: 'gh api timed out', lastGoodAt: new Date(Date.now() - 42e3 * 60).toISOString(), observedAt: new Date(Date.now() - 60e3).toISOString() },
  { id: 'devin', state: 'ok', used: 56, plan: 'Pro', windows: [{ label: 'week', used: 28 }, { label: 'day', used: 56 }], details: [{ label: 'Day', value: '56% used', resetAt: later(6.3 * H) }, { label: 'Week', value: '28% used', resetAt: later(2 * 24 * H + 6.3 * H) }] },
  { id: 'antigravity', state: 'ok', used: 0, reset5h: later(4.9 * H), groups: [{ name: 'Gemini 3 Pro', windows: [{ label: '5h', used: 0 }] }, { name: 'Claude Sonnet 4.5', windows: [{ label: '5h', used: 0 }] }, { name: 'GPT-OSS 120B', windows: [{ label: '5h', used: 0 }] }] }
];

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 480, height: 800 } });
  await page.addInitScript({ path: 'C:/Users/Bennett/Documents/PROJECT/agent-quota/bloom/e2e/tauri-mock.js' });
  await page.goto('http://localhost:1420/');
  await page.waitForSelector('div.bloom', { timeout: 15000 });
  await page.waitForTimeout(1100);
  const emit = (readings) => page.evaluate((p) => window.__emit('quota-snapshot', p), readings);
  const openQs = async () => {
    await page.locator('div.bloom').click();
    for (let i = 0; i < 5 && (await page.locator('[data-testid="quota-space"]').count()) === 0; i++) {
      const n = page.locator('button.panel-arrow.next');
      if ((await n.count()) === 0) break;
      await n.click(); await page.waitForTimeout(300);
    }
    await page.waitForSelector('[data-testid="quota-space"]', { timeout: 8000 });
    await page.waitForTimeout(900);
  };
  const metrics = () => page.evaluate(() => {
    const q = (s) => document.querySelector(s);
    const r = (e) => e ? e.getBoundingClientRect().toJSON() : null;
    const bloom = q('div.bloom'), qs = q('.qs');
    const cells = [...document.querySelectorAll('.qs-cell')].map(c => {
      const top = c.querySelector('.qs-cell-top'), pct = c.querySelector('.qs-pct');
      const cr = c.getBoundingClientRect(), tr = top.getBoundingClientRect(), pr = pct.getBoundingClientRect();
      return { id: c.dataset.testid, cellW: cr.width, topW: tr.width, topOverflow: tr.width > cr.width - 4, pctRight: pr.right, cellRight: cr.right, pctClipped: pr.right > cr.right + 0.5, name: c.querySelector('.qs-name').textContent, basis: c.querySelector('.qs-basis').textContent, pct: pct.textContent };
    });
    // element overlap check between adjacent cells' inner content
    const marks = [...document.querySelectorAll('.qs-mark svg, .qs-mark img')].map(m => m.getBoundingClientRect().toJSON());
    return {
      bloom: r(bloom), bloomStyle: { w: bloom.style.width, h: bloom.style.height, t: bloom.style.transform },
      qs: r(qs), viewport: r(q('.quota-viewport')),
      footer: r(q('.qs-footer')), failed: q('.qs-failed')?.textContent,
      footerInPill: q('.qs-footer') ? (q('.qs-footer').getBoundingClientRect().bottom <= bloom.getBoundingClientRect().bottom + 0.5) : null,
      cells, marksCount: marks.length, marksHaveSize: marks.every(m => m.width > 4 && m.height > 4),
      selectedDetail: q('.qs-detail')?.innerText.slice(0, 400),
      scroll: qs ? { scrollW: qs.scrollWidth, clientW: qs.clientWidth, xOverflow: qs.scrollWidth > qs.clientWidth } : null,
      ringSvgStroke: getComputedStyle(document.querySelector('.qs-ring-fill') || document.querySelector('.qs-ring-track')).strokeWidth
    };
  });

  // A: all seven agents
  await emit(SEVEN); await openQs();
  console.log('=== 7AGENTS ==='); console.log(JSON.stringify(await metrics(), null, 1));
  await page.locator('[data-testid="quota-space"]').screenshot({ path: OUT + '/mock-7agents.png' });
  await page.screenshot({ path: OUT + '/mock-7agents-viewport.png' });
  // B: select antigravity, expand groups
  await page.locator('[data-testid="quota-cell-antigravity"]').click(); await page.waitForTimeout(200);
  const tg = page.locator('[data-testid="quota-groups-toggle"]');
  if (await tg.count()) { await tg.click(); await page.waitForTimeout(400); }
  await page.locator('[data-testid="quota-space"]').screenshot({ path: OUT + '/mock-groups.png' });
  // C: select stale copilot + failed claude
  await page.locator('[data-testid="quota-cell-copilot"]').click(); await page.waitForTimeout(200);
  await page.locator('[data-testid="quota-space"]').screenshot({ path: OUT + '/mock-stale.png' });
  await page.locator('[data-testid="quota-cell-claude"]').click(); await page.waitForTimeout(200);
  await page.locator('[data-testid="quota-space"]').screenshot({ path: OUT + '/mock-failed.png' });
  // D: empty
  await emit([]); await page.waitForTimeout(400);
  await page.locator('[data-testid="quota-space"]').screenshot({ path: OUT + '/mock-empty.png' });
  await page.screenshot({ path: OUT + '/mock-empty-viewport.png' });
  console.log('=== EMPTY ==='); console.log(JSON.stringify(await metrics(), null, 1));
  await browser.close();
  process.exit(0);
})().catch(e => { console.log('FAIL', e.message); process.exit(1); });
