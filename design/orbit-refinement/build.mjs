import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const directory = path.dirname(fileURLToPath(import.meta.url));
const project = path.resolve(directory, '../..');
const paths = fs.readFileSync(path.join(project, 'internal/marks/markpaths.go'), 'utf8');
const mark = name => {
  const match = paths.match(new RegExp(`${name}\\s*= "([^"]+)"`));
  if (!match) throw new Error(`Missing existing artwork: ${name}`);
  return match[1];
};
const svg = body => `data:image/svg+xml;base64,${Buffer.from(body).toString('base64')}`;
const asset = (name, mime) => `data:${mime};base64,${fs.readFileSync(path.join(project, 'internal/marks/assets', name)).toString('base64')}`;
const marks = {
  claude: svg(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="white" d="${mark('markClaude')}"/></svg>`),
  kiro: asset('kiro.svg', 'image/svg+xml'),
  codex: svg(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="3 3 18 18"><defs><linearGradient id="g" x2=".5" y2="1"><stop stop-color="#b1a7ff"/><stop offset="1" stop-color="#3941ff"/></linearGradient></defs><path fill="url(#g)" d="${mark('markCodex')}"/><path fill="white" d="${mark('markCodexGlyph')}"/></svg>`),
  cursor: svg(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="white" fill-rule="evenodd" d="${mark('markCursor')}"/></svg>`),
  copilot: asset('copilot-24.svg', 'image/svg+xml'),
  devin: asset('devin.svg', 'image/svg+xml'),
  antigravity: asset('antigravity.png', 'image/png'),
};
fs.writeFileSync(path.join(directory, 'marks.js'), `// Generated from the application's licensed agent artwork.\nconst MARKS = ${JSON.stringify(marks)};\n`);
console.log('Built agent artwork from the native application assets.');
