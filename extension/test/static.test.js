// What can be checked by reading the files: the manifest's permissions and
// content security policy, what the sources never do, the licence copy, and
// that the README lists every selector the adapters use.

import assert from 'node:assert/strict';
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

import { SETTING_KEYS } from '../src/background/settings.js';
import { ADAPTERS, generic } from '../src/content/adapters/index.js';
import { EVENT_TYPES } from '../src/content/controller.js';
import { SITES } from '../src/shared/sites.js';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const read = (...parts) => readFileSync(join(root, ...parts), 'utf8');
const manifest = JSON.parse(read('manifest.json'));
const siteMatches = Object.values(SITES).map((origin) => `${origin}/*`);

function files(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) files(path, out);
    else out.push(path);
  }
  return out;
}
const shipped = [...files(join(root, 'src')), ...files(join(root, 'options'))];
const rel = (path) => relative(root, path).replace(/\\/g, '/');

test('manifest: version 3, the three sites and nothing wider', () => {
  assert.equal(manifest.manifest_version, 3);
  assert.deepEqual(manifest.host_permissions, siteMatches);
  // The second sets the vendors' workspace headers (tenant-rules.js). It
  // acts only where host access is granted, the three sites, and warns of
  // nothing at install.
  assert.deepEqual(manifest.permissions, ['storage', 'declarativeNetRequestWithHostAccess']);
  assert.equal(manifest.content_scripts.length, 1);
  assert.deepEqual(manifest.content_scripts[0].matches, siteMatches);
  assert.equal(manifest.content_scripts[0].all_frames, false);
  assert.deepEqual(manifest.web_accessible_resources.map((r) => r.matches), [siteMatches]);
  // The Tamga origin is asked for at configuration time, not granted up front.
  assert.deepEqual(manifest.optional_host_permissions, ['https://*/*', 'http://localhost/*', 'http://127.0.0.1/*']);
  for (const key of ['externally_connectable', 'optional_permissions', 'update_url', 'oauth2', 'key']) {
    assert.equal(manifest[key], undefined, key);
  }
});

test('manifest: a strict content security policy', () => {
  const csp = manifest.content_security_policy.extension_pages;
  const directives = Object.fromEntries(csp.split(';').map((d) => d.trim().split(/\s+/)).map(([name, ...values]) => [name, values]));
  assert.deepEqual(directives['default-src'], ["'none'"]);
  assert.deepEqual(directives['script-src'], ["'self'"]);
  assert.deepEqual(directives['style-src'], ["'self'"]);
  assert.deepEqual(directives['object-src'], ["'none'"]);
  assert.deepEqual(directives['base-uri'], ["'none'"]);
  assert.deepEqual(directives['form-action'], ["'none'"]);
  assert.ok(!/unsafe-|wasm|blob:|data:/.test(csp));
});

test('manifest: every file it names exists, and the worker is a module', () => {
  const named = [
    manifest.background.service_worker,
    ...manifest.content_scripts.flatMap((c) => c.js),
    manifest.options_ui.page,
    manifest.storage.managed_schema,
  ];
  for (const file of named) assert.ok(existsSync(join(root, file)), file);
  assert.equal(manifest.background.type, 'module');
  // What the loader imports has to be reachable from the three sites.
  assert.deepEqual(manifest.web_accessible_resources[0].resources, ['src/content/*', 'src/shared/*']);
});

test('managed_schema.json describes exactly the settings the code reads', () => {
  const schema = JSON.parse(read('managed_schema.json'));
  assert.deepEqual(Object.keys(schema.properties).sort(), [...SETTING_KEYS].sort());
  assert.deepEqual(schema.properties.failMode.enum, ['closed', 'open']);
  assert.deepEqual(schema.properties.sites.items.enum, Object.keys(SITES));
});

const NEVER = [
  ['innerHTML', /\binnerHTML\b/],
  ['outerHTML', /\bouterHTML\b/],
  ['insertAdjacentHTML', /insertAdjacentHTML/],
  ['document.write', /document\.write/],
  ['eval', /\beval\s*\(/],
  ['new Function', /new\s+Function\b/],
  ['localStorage', /\blocalStorage\b/],
  ['sessionStorage', /\bsessionStorage\b/],
  ['XMLHttpRequest', /XMLHttpRequest/],
  ['WebSocket', /\bWebSocket\b/],
  ['EventSource', /\bEventSource\b/],
  ['sendBeacon', /sendBeacon/],
  ['importScripts', /importScripts/],
  ['console', /\bconsole\./],
  ['postMessage', /postMessage/],
];

test('the sources never build markup from strings, never log, never use the page\'s storage, and have one way out', () => {
  for (const path of shipped.filter((p) => p.endsWith('.js'))) {
    const code = read(rel(path)).replace(/^\s*\/\/.*$/gm, '');
    for (const [name, pattern] of NEVER) assert.ok(!pattern.test(code), `${rel(path)} uses ${name}`);
  }
});

test('fetch is called in one file, the service worker, and nowhere in a content script', () => {
  const callers = shipped
    .filter((p) => p.endsWith('.js'))
    .filter((p) => /(?<![.\w])fetch\s*\(/.test(read(rel(p)).replace(/^\s*\/\/.*$/gm, '')))
    .map(rel);
  assert.deepEqual(callers, ['src/background/service-worker.js']);

  for (const path of shipped.filter((p) => rel(p).startsWith('src/content/') || rel(p).startsWith('src/shared/'))) {
    const code = read(rel(path)).replace(/^\s*\/\/.*$/gm, '');
    assert.ok(!/chrome\.storage/.test(code), `${rel(path)} reads chrome.storage`);
    assert.ok(!/fetchImpl|tamgaKey|X-Tamga-Key/.test(code), `${rel(path)} knows about the key or the network`);
  }
});

test('the only addresses written into the sources are the three sites', () => {
  const allowed = new Set(Object.values(SITES));
  for (const path of shipped.filter((p) => p.endsWith('.js'))) {
    const code = read(rel(path)).replace(/^\s*\/\/.*$/gm, '');
    for (const [url] of code.matchAll(/https?:\/\/[^\s'"`)]+/g)) {
      assert.ok(allowed.has(url), `${rel(path)} names ${url}`);
    }
  }
  // The pages load nothing from anywhere else either.
  const html = read('options', 'options.html');
  assert.ok(!/<script[^>]+src="(?!options\.js")/.test(html));
  assert.ok(!/<link[^>]+href="(?!options\.css")/.test(html));
  assert.ok(!/\son\w+=/.test(html), 'no inline event handlers');
});

test('the LICENSE here is the repository\'s', (t) => {
  const mine = join(root, 'LICENSE');
  const theirs = resolve(root, '..', 'LICENSE');
  assert.ok(existsSync(mine), 'extension/LICENSE is missing');
  const text = (p) => readFileSync(p, 'utf8').replace(/\r\n/g, '\n');
  assert.ok(text(mine).includes('GNU AFFERO GENERAL PUBLIC LICENSE'));
  assert.equal(JSON.parse(read('package.json')).license, 'AGPL-3.0-only');
  if (!existsSync(theirs)) {
    t.skip('repository root LICENSE not present (the extension was copied out of the repository)');
    return;
  }
  assert.equal(text(mine), text(theirs), 'extension/LICENSE differs from the repository root LICENSE; copy the root file over it');
});

test('the README lists every selector, says none is verified, and carries the date', () => {
  const readme = read('README.md');
  assert.match(readme, /2026-10-11/);
  assert.match(readme, /not verified|unverified/i);
  for (const adapter of [...Object.values(ADAPTERS), generic]) {
    for (const selector of [...adapter.selectors.composer, ...adapter.selectors.sendButton, ...adapter.selectors.fileInput]) {
      assert.ok(readme.includes(`\`${selector}\``), `README.md does not list ${adapter.id}: ${selector}`);
    }
  }
});

test('the loader listens for every event the controller handles', () => {
  const list = /for \(const type of (\[[^\]]*\])\)/.exec(read('src', 'content', 'loader.js'));
  assert.ok(list, 'loader.js has no list of event types');
  assert.deepEqual(JSON.parse(list[1].replace(/'/g, '"')), [...EVENT_TYPES]);
});

test('the versions agree', () => {
  assert.equal(manifest.version, JSON.parse(read('package.json')).version);
});

test('manifest: every icon it names is a PNG of the size it is named for', () => {
  const named = { ...manifest.icons };
  for (const [size, file] of Object.entries(manifest.action.default_icon)) assert.equal(named[size], file);
  assert.deepEqual(Object.keys(named).sort((a, b) => a - b), ['16', '32', '48', '128']);
  for (const [size, file] of Object.entries(named)) {
    const bytes = readFileSync(join(root, file));
    assert.equal(bytes.subarray(1, 4).toString('latin1'), 'PNG', file);
    // Width and height are the two big-endian words that open the IHDR chunk.
    assert.equal(bytes.readUInt32BE(16), Number(size), `${file} width`);
    assert.equal(bytes.readUInt32BE(20), Number(size), `${file} height`);
  }
});
