// Drive the live Bloom notch: open the quota space and leave it there.
const { chromium } = require('C:/Users/Bennett/Documents/PROJECT/agent-quota/.scratch/qa/node_modules/playwright-core');
const wait = (ms) => new Promise(r => setTimeout(r, ms));
(async () => {
  const browser = await chromium.connectOverCDP('http://localhost:9222');
  let page;
  for (const p of browser.contexts().flatMap(c => c.pages())) {
    if (await p.evaluate(() => !!document.querySelector('.bloom')).catch(() => false)) { page = p; break; }
  }
  const state = process.argv[2] || 'quota';
  await page.evaluate(() => window.__TAURI_INTERNALS__.invoke('set_notch_hovered', { hovered: true }).catch(() => {}));
  await wait(400);
  const b = await page.evaluate(() => { const r = document.querySelector('div.bloom').getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 }; });
  await page.mouse.move(b.x, b.y); await wait(300);
  await page.mouse.click(b.x, b.y); await wait(800);
  for (let i = 0; i < 5 && !(await page.evaluate(() => !!document.querySelector('.quota-viewport'))); i++) {
    await page.evaluate(() => document.querySelector('button.panel-arrow.next')?.click());
    await wait(500);
  }
  if (state === 'groups') {
    await page.evaluate(() => document.querySelector('[data-testid="quota-cell-antigravity"]')?.click());
    await wait(300);
    await page.evaluate(() => document.querySelector('[data-testid="quota-groups-toggle"]')?.click());
    await wait(600);
  }
  await wait(1200);
  console.log('READY', state);
  process.exit(0);
})().catch(e => { console.log('FAIL', e.message); process.exit(1); });
