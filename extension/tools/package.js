// Packs the extension for Chromium (Chrome, Edge) and for Firefox from this
// one source tree. No dependencies.
//
//   node tools/package.js [chromium|firefox|all] [--out <dir>] [--gecko-id <id>]
//
// For each browser it writes <out>/<browser>/, the extension as a directory
// (to load unpacked, or to pack with chrome --pack-extension), and
// <out>/tamga-<browser>-<version>.zip (for Firefox, the file to have signed).
// <out> is extension/dist unless given.
//
// manifest.json in the source tree is the Chromium one. The Firefox manifest
// is made from it here, and differs only where Firefox needs it to:
//
// - background: Firefox has no extension service workers. The same module
//   runs there as an event page (background.scripts).
// - browser_specific_settings.gecko: the add-on ID, which Firefox needs for
//   managed storage and enterprise policy, the oldest Firefox it is for, and
//   the data it sends off the device.
// - no minimum_chrome_version, and no storage.managed_schema: Firefox reads
//   policy values from the "3rdparty" section of its enterprise policy and
//   does not use a schema for them.
// - web_accessible_resources names the adapters' directory as well, rather
//   than relying on a wildcard to reach into a subdirectory.

import { mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deflateRawSync } from 'node:zlib';

export const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
export const BROWSERS = Object.freeze(['chromium', 'firefox']);

// The add-on ID of the project's own Firefox build. An organisation that has
// its copy signed by Mozilla needs an ID of its own (--gecko-id): an ID is
// unique across addons.mozilla.org, and the project has registered none.
export const GECKO_ID = 'chat-sites@tamgaproxy.com';
// Firefox ESR 140: the first release with data_collection_permissions.
export const GECKO_MIN_VERSION = '140.0';

// What goes into a package: the extension and its licence, not its tests,
// its documentation or this script.
const COMMON = ['manifest.json', 'LICENSE', 'icons', 'options', 'src'];
const ONLY = { chromium: ['managed_schema.json'], firefox: [] };

const GECKO_ID_PATTERN = /^(?:[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+|\{[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\})$/;

/** The manifest for one browser, made from the source tree's manifest. */
export function manifestFor(source, browser, { geckoId = GECKO_ID } = {}) {
  const m = structuredClone(source);
  if (browser === 'chromium') return m;
  if (browser !== 'firefox') throw new Error(`unknown browser: ${browser}`);
  if (!GECKO_ID_PATTERN.test(geckoId)) throw new Error(`not a Firefox add-on ID: ${geckoId}`);

  m.background = { scripts: [source.background.service_worker], type: source.background.type };
  delete m.minimum_chrome_version;
  delete m.storage;
  m.web_accessible_resources = source.web_accessible_resources.map((entry) => ({
    ...entry,
    resources: [...new Set(entry.resources.flatMap((r) => (r === 'src/content/*' ? [r, 'src/content/adapters/*'] : [r])))],
  }));
  m.browser_specific_settings = {
    gecko: {
      id: geckoId,
      strict_min_version: GECKO_MIN_VERSION,
      // The text of what is pasted, sent or attached on the chat sites goes
      // to the organisation's own Tamga server, which is off the device.
      data_collection_permissions: { required: ['websiteContent'] },
    },
  };
  return m;
}

function walk(path, out) {
  if (statSync(path).isDirectory()) {
    for (const name of readdirSync(path).sort()) walk(join(path, name), out);
  } else {
    out.push(path);
  }
  return out;
}

/** The files of one browser's package: [{ name, data }], manifest first, then by name. */
export function packageFiles(browser, options = {}, root = ROOT) {
  const source = JSON.parse(readFileSync(join(root, 'manifest.json'), 'utf8'));
  const files = [];
  for (const entry of [...COMMON, ...ONLY[browser]]) {
    for (const path of walk(join(root, entry), [])) {
      const name = relative(root, path).replace(/\\/g, '/');
      if (name === 'manifest.json') continue;
      files.push({ name, data: readFileSync(path) });
    }
  }
  files.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  const manifest = `${JSON.stringify(manifestFor(source, browser, options), null, 2)}\n`;
  return [{ name: 'manifest.json', data: Buffer.from(manifest, 'utf8') }, ...files];
}

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    table[n] = c >>> 0;
  }
  return table;
})();

export function crc32(buf) {
  let c = 0xffffffff;
  for (const byte of buf) c = CRC_TABLE[(c ^ byte) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

// Every entry carries the same time (1980-01-01 00:00), so that the same
// sources make the same archive.
const DOS_TIME = 0;
const DOS_DATE = (0 << 9) | (1 << 5) | 1;
const UTF8_NAMES = 0x0800;

/** A zip archive of `entries` ([{ name, data }]), deflated. */
export function zip(entries) {
  const locals = [];
  const centrals = [];
  let offset = 0;
  for (const { name, data } of entries) {
    const nameBytes = Buffer.from(name, 'utf8');
    const packed = deflateRawSync(data, { level: 9 });
    const crc = crc32(data);

    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(UTF8_NAMES, 6);
    local.writeUInt16LE(8, 8);
    local.writeUInt16LE(DOS_TIME, 10);
    local.writeUInt16LE(DOS_DATE, 12);
    local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(packed.length, 18);
    local.writeUInt32LE(data.length, 22);
    local.writeUInt16LE(nameBytes.length, 26);
    local.writeUInt16LE(0, 28);
    locals.push(local, nameBytes, packed);

    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(20, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt16LE(UTF8_NAMES, 8);
    central.writeUInt16LE(8, 10);
    central.writeUInt16LE(DOS_TIME, 12);
    central.writeUInt16LE(DOS_DATE, 14);
    central.writeUInt32LE(crc, 16);
    central.writeUInt32LE(packed.length, 20);
    central.writeUInt32LE(data.length, 24);
    central.writeUInt16LE(nameBytes.length, 28);
    central.writeUInt32LE(offset, 42);
    centrals.push(central, nameBytes);

    offset += local.length + nameBytes.length + packed.length;
  }
  const directory = Buffer.concat(centrals);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(directory.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, directory, end]);
}

/** Writes one browser's directory and zip under `out`. Returns their paths. */
export function build(browser, { out = join(ROOT, 'dist'), geckoId } = {}) {
  const files = packageFiles(browser, geckoId ? { geckoId } : {});
  const version = JSON.parse(files[0].data.toString('utf8')).version;
  const dir = join(out, browser);
  rmSync(dir, { recursive: true, force: true });
  for (const { name, data } of files) {
    mkdirSync(dirname(join(dir, name)), { recursive: true });
    writeFileSync(join(dir, name), data);
  }
  const archive = join(out, `tamga-${browser}-${version}.zip`);
  mkdirSync(out, { recursive: true });
  writeFileSync(archive, zip(files));
  return { dir, archive, files: files.map((f) => f.name) };
}

function main(argv) {
  let which = 'all';
  const options = {};
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === '--out') options.out = resolve(argv[++i] ?? '');
    else if (arg === '--gecko-id') options.geckoId = argv[++i];
    else if (arg === 'all' || BROWSERS.includes(arg)) which = arg;
    else throw new Error(`usage: node tools/package.js [chromium|firefox|all] [--out <dir>] [--gecko-id <id>] (not: ${arg})`);
  }
  for (const browser of which === 'all' ? BROWSERS : [which]) {
    const { dir, archive } = build(browser, options);
    process.stdout.write(`${browser}: ${dir}\n${browser}: ${archive}\n`);
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main(process.argv.slice(2));
  } catch (err) {
    process.stderr.write(`${err.message}\n`);
    process.exitCode = 1;
  }
}
