const { chromium } = require('../qa/node_modules/playwright-core');
const fs = require('node:fs');
const path = require('node:path');
(async () => {
  const browser = await chromium.connectOverCDP('http://localhost:9222', { timeout: 10000 });
  let page;
  for (const candidate of browser.contexts().flatMap(c => c.pages())) {
    if (await candidate.evaluate(() => !!document.querySelector('div.bloom')).catch(() => false)) { page = candidate; break; }
  }
  if (!page) throw new Error('Native main WebView missing');
  const state = await page.evaluate(() => {
    const rect = s => document.querySelector(s)?.getBoundingClientRect().toJSON();
    return { url: location.href, viewport: {w:innerWidth,h:innerHeight,dpr:devicePixelRatio}, bloom:rect('div.bloom'), qs:rect('.qs'), footer:rect('.qs-footer'), text:document.querySelector('div.bloom')?.innerText, visible:document.visibilityState, scale:localStorage.getItem('bloom-scale') };
  });
  state.notchMode = await page.evaluate(async () => (await window.__TAURI_INTERNALS__.invoke('load_settings'))['bloom-notch-mode']);
  console.log(JSON.stringify(state));
  fs.writeFileSync(path.join(__dirname,'confirm-native-state.json'), JSON.stringify(state,null,2));
  if (state.bloom?.y >= 0 && state.bloom?.width > 40) await page.locator('div.bloom').screenshot({path:path.join(__dirname,'confirm-native-app.png'),timeout:5000});
  await browser.close();
})().catch(e => { console.error(e); process.exitCode=1; });
