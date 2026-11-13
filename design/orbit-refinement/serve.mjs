import http from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const directory = path.dirname(fileURLToPath(import.meta.url));
const project = path.resolve(directory, '../..');
const routes = new Map([
  ['/', [path.join(directory, 'index.html'), 'text/html; charset=utf-8']],
  ...['study.css', 'study.js', 'marks.js'].map(name => ['/' + name, [path.join(directory, name), name.endsWith('.css') ? 'text/css; charset=utf-8' : 'text/javascript; charset=utf-8']]),
  ...['Regular', 'Medium', 'SemiBold'].map(weight => [`/fonts/Inter-${weight}.ttf`, [path.join(project, `internal/text/assets/Inter-${weight}.ttf`), 'font/ttf']]),
]);
const server = http.createServer((request, response) => {
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    response.writeHead(405).end();
    return;
  }
  const target = routes.get(new URL(request.url, 'http://127.0.0.1').pathname);
  if (!target) {
    response.writeHead(404).end('Not found');
    return;
  }
  try {
    const body = fs.readFileSync(target[0]);
    response.writeHead(200, {
      'Content-Type': target[1],
      'Cache-Control': 'no-store',
      'X-Content-Type-Options': 'nosniff',
      'Content-Security-Policy': "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; font-src 'self'; connect-src 'none'; frame-ancestors 'none'",
    });
    response.end(request.method === 'HEAD' ? undefined : body);
  } catch {
    response.writeHead(404).end('Not found');
  }
});
server.listen(4188, '127.0.0.1', () => console.log('BOARD_URL: http://127.0.0.1:4188/'));
