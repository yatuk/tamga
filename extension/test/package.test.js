// The packaging step: what each browser's package holds, the Firefox
// manifest made from the Chromium one, and an archive that unpacks to the
// same files. Firefox itself is not run here; what it would make of the
// manifest is checked against what Firefox documents.

import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, posix } from 'node:path';
import { test } from 'node:test';
import { inflateRawSync } from 'node:zlib';

import { GECKO_ID, GECKO_MIN_VERSION, ROOT, build, crc32, manifestFor, packageFiles, zip } from '../tools/package.js';

const source = JSON.parse(readFileSync(join(ROOT, 'manifest.json'), 'utf8'));
const firefox = manifestFor(source, 'firefox');

/** Reads a zip's central directory back: [{ name, data }]. */
function unzip(buf) {
  const end = buf.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  assert.ok(end >= 0, 'no end of central directory');
  const count = buf.readUInt16LE(end + 10);
  let at = buf.readUInt32LE(end + 16);
  const out = [];
  for (let i = 0; i < count; i++) {
    assert.equal(buf.readUInt32LE(at), 0x02014b50);
    const method = buf.readUInt16LE(at + 10);
    const crc = buf.readUInt32LE(at + 16);
    const size = buf.readUInt32LE(at + 20);
    const nameLength = buf.readUInt16LE(at + 28);
    const offset = buf.readUInt32LE(at + 42);
    const name = buf.subarray(at + 46, at + 46 + nameLength).toString('utf8');
    const localName = buf.readUInt16LE(offset + 26);
    const localExtra = buf.readUInt16LE(offset + 28);
    const start = offset + 30 + localName + localExtra;
    assert.equal(method, 8, name);
    const data = inflateRawSync(buf.subarray(start, start + size));
    assert.equal(crc32(data), crc, `${name}: crc`);
    out.push({ name, data });
    at += 46 + nameLength + buf.readUInt16LE(at + 30) + buf.readUInt16LE(at + 32);
  }
  return out;
}

test('crc32 is the zip one', () => {
  assert.equal(crc32(Buffer.from('123456789')), 0xcbf43926);
  assert.equal(crc32(Buffer.alloc(0)), 0);
});

test('chromium: the manifest is the source tree\'s, and the package is the extension without its tests or docs', () => {
  const files = packageFiles('chromium');
  assert.deepEqual(JSON.parse(files[0].data.toString('utf8')), source);
  const names = files.map((f) => f.name);
  assert.equal(names[0], 'manifest.json');
  for (const name of names) {
    assert.ok(/^(manifest\.json|managed_schema\.json|LICENSE|icons\/|options\/|src\/)/.test(name), `${name} should not be packed`);
  }
  for (const needed of ['managed_schema.json', 'LICENSE', 'src/content/loader.js', 'src/content/files.js', 'src/background/service-worker.js', 'options/options.html', 'icons/icon-128.png']) {
    assert.ok(names.includes(needed), needed);
  }
});

test('firefox: an event page in place of the service worker, the same module', () => {
  assert.deepEqual(firefox.background, { scripts: ['src/background/service-worker.js'], type: 'module' });
  assert.equal(firefox.background.service_worker, undefined);
});

test('firefox: an add-on ID, the oldest release it is for, and the data it sends', () => {
  assert.deepEqual(firefox.browser_specific_settings, {
    gecko: { id: GECKO_ID, strict_min_version: GECKO_MIN_VERSION, data_collection_permissions: { required: ['websiteContent'] } },
  });
  assert.equal(manifestFor(source, 'firefox', { geckoId: 'tamga@corp.example.invalid' }).browser_specific_settings.gecko.id, 'tamga@corp.example.invalid');
  assert.equal(manifestFor(source, 'firefox', { geckoId: '{12345678-1234-1234-1234-123456789abc}' }).browser_specific_settings.gecko.id, '{12345678-1234-1234-1234-123456789abc}');
  for (const bad of ['', 'no-at-sign', 'a b@c.d', '{not-a-uuid}']) {
    assert.throws(() => manifestFor(source, 'firefox', { geckoId: bad }), /not a Firefox add-on ID/, bad);
  }
  assert.throws(() => manifestFor(source, 'safari'), /unknown browser/);
});

test('firefox: what Chrome alone reads is left out, and everything else is as in the source', () => {
  assert.equal(firefox.minimum_chrome_version, undefined);
  assert.equal(firefox.storage, undefined);
  for (const key of ['manifest_version', 'version', 'permissions', 'host_permissions', 'optional_host_permissions', 'content_scripts', 'options_ui', 'action', 'icons', 'content_security_policy']) {
    assert.deepEqual(firefox[key], source[key], key);
  }
  assert.ok(!packageFiles('firefox').some((f) => f.name === 'managed_schema.json'));
});

// Every module the content script loads has to be web-accessible. Read
// strictly, a "*" in a resource pattern does not reach into a subdirectory.
function imported(file, seen = new Set()) {
  if (seen.has(file)) return seen;
  seen.add(file);
  const code = readFileSync(join(ROOT, file), 'utf8');
  for (const [, spec] of code.matchAll(/(?:from|import)\s*\(?\s*'(\.{1,2}\/[^']+)'/g)) {
    imported(posix.normalize(posix.join(posix.dirname(file), spec)), seen);
  }
  return seen;
}

test('firefox: every module the content script loads is web-accessible, without a wildcard reaching into a subdirectory', () => {
  const modules = [...imported('src/content/main.js')];
  assert.ok(modules.includes('src/content/adapters/claude.js'));
  assert.ok(modules.includes('src/content/files.js'));
  const patterns = firefox.web_accessible_resources.flatMap((r) => r.resources);
  const strict = (pattern, file) => new RegExp(`^${pattern.replace(/[.]/g, '\\.').replace(/\*/g, '[^/]*')}$`).test(file);
  for (const file of modules) assert.ok(patterns.some((p) => strict(p, file)), `${file} is not web-accessible in Firefox`);
  assert.deepEqual(firefox.web_accessible_resources.map((r) => r.matches), source.web_accessible_resources.map((r) => r.matches));
});

test('the extension code reaches the browser through one name that Firefox and Chrome both have', () => {
  for (const file of ['src/background/service-worker.js', 'src/content/main.js', 'options/options.js']) {
    const code = readFileSync(join(ROOT, file), 'utf8').replace(/^\s*\/\/.*$/gm, '');
    assert.ok(!/\bchrome\./.test(code), `${file} calls chrome.* directly`);
    assert.match(code, /import \{ ext \} from '[./]+\/(?:src\/)?shared\/api\.js'/, file);
  }
  const loader = readFileSync(join(ROOT, 'src/content/loader.js'), 'utf8');
  assert.match(loader, /globalThis\.browser \?\? globalThis\.chrome/);
});

test('the settings page asks for the host permission before it waits for anything', () => {
  // Firefox honours permissions.request only from the click's handler before
  // its first await: everything Save needs is read when the page is drawn.
  const code = readFileSync(join(ROOT, 'options/options.js'), 'utf8');
  const save = code.slice(code.indexOf('async function save('), code.indexOf('await ext.permissions.request('));
  assert.ok(save.length > 0);
  assert.ok(!/\bawait\b/.test(save), 'save() waits for something before permissions.request');
});

test('build writes a directory and an archive per browser, and the archive unpacks to the same files', () => {
  const out = mkdtempSync(join(tmpdir(), 'tamga-ext-'));
  try {
    for (const browser of ['chromium', 'firefox']) {
      const { dir, archive, files } = build(browser, { out });
      assert.equal(archive, join(out, `tamga-${browser}-${source.version}.zip`));
      const entries = unzip(readFileSync(archive));
      assert.deepEqual(entries.map((e) => e.name), files);
      for (const { name, data } of entries) {
        assert.ok(existsSync(join(dir, name)), name);
        assert.ok(readFileSync(join(dir, name)).equals(data), `${name} differs between the directory and the archive`);
      }
      const manifest = JSON.parse(readFileSync(join(dir, 'manifest.json'), 'utf8'));
      for (const named of [manifest.background.service_worker, ...(manifest.background.scripts ?? []), manifest.options_ui.page, ...manifest.content_scripts.flatMap((c) => c.js)].filter(Boolean)) {
        assert.ok(existsSync(join(dir, named)), `${browser}: the manifest names ${named}, which is not in the package`);
      }
      assert.ok(existsSync(dirname(archive)));
    }
    // The same sources make the same archive.
    const first = readFileSync(build('firefox', { out }).archive);
    assert.ok(readFileSync(build('firefox', { out }).archive).equals(first));
  } finally {
    rmSync(out, { recursive: true, force: true });
  }
});

test('zip: an empty file and a name outside ASCII survive', () => {
  const entries = [{ name: 'empty.txt', data: Buffer.alloc(0) }, { name: 'dir/gönder.txt', data: Buffer.from('x') }];
  assert.deepEqual(unzip(zip(entries)).map((e) => [e.name, e.data.toString()]), [['empty.txt', ''], ['dir/gönder.txt', 'x']]);
});
