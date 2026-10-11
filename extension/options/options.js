// The settings page. It writes to chrome.storage.local and asks the browser
// for permission to reach the Tamga origin; it makes no network request
// itself. A setting that policy fixed is shown and cannot be changed. The
// stored key is never put back into the page.

import { loadSettings, parseTamgaUrl } from '../src/background/settings.js';
import { MSG_TEST } from '../src/shared/protocol.js';
import { reasonText } from '../src/content/decide.js';

const form = document.getElementById('settings');
const status = document.getElementById('status');
const field = (name) => form.elements.namedItem(name);
const siteBoxes = () => [...form.querySelectorAll('input[name="sites"]')];

let managedKeys = [];

function say(text) {
  status.textContent = text;
}

async function render() {
  const loaded = await loadSettings({ managed: chrome.storage.managed, local: chrome.storage.local });
  const { settings } = loaded;
  managedKeys = loaded.managedKeys;

  field('tamgaUrl').value = settings.tamgaUrl;
  field('tamgaKey').value = '';
  field('tamgaKey').placeholder = settings.tamgaKey ? 'A key is stored' : 'tk_...';
  field('failMode').value = settings.failMode;
  field('autoRedact').checked = settings.autoRedact;
  for (const box of siteBoxes()) box.checked = settings.sites.includes(box.value);

  for (const name of ['tamgaUrl', 'tamgaKey', 'failMode', 'autoRedact']) {
    field(name).disabled = managedKeys.includes(name);
  }
  for (const box of siteBoxes()) box.disabled = managedKeys.includes('sites');
  for (const mark of document.querySelectorAll('.managed')) {
    mark.hidden = !managedKeys.includes(mark.dataset.for);
  }
  document.getElementById('managed-note').hidden = managedKeys.length === 0;
  document.getElementById('key-hint').hidden = managedKeys.includes('tamgaKey');
}

async function save(event) {
  event.preventDefault();
  const next = {};
  const free = (name) => !managedKeys.includes(name);

  let origin = null;
  if (free('tamgaUrl')) {
    const value = field('tamgaUrl').value.trim();
    const target = parseTamgaUrl(value);
    if (!target) {
      say('The Tamga URL has to be an https address (http only for localhost), with no query and no credentials in it.');
      return;
    }
    next.tamgaUrl = value;
    origin = target.origin;
  } else {
    const { settings } = await loadSettings({ managed: chrome.storage.managed, local: chrome.storage.local });
    origin = parseTamgaUrl(settings.tamgaUrl)?.origin ?? null;
  }
  if (free('tamgaKey') && field('tamgaKey').value.trim() !== '') next.tamgaKey = field('tamgaKey').value.trim();
  if (free('failMode')) next.failMode = field('failMode').value;
  if (free('autoRedact')) next.autoRedact = field('autoRedact').checked;
  if (free('sites')) next.sites = siteBoxes().filter((box) => box.checked).map((box) => box.value);

  // Asked from the click on Save: the browser shows its own prompt naming
  // the one origin.
  let granted = false;
  if (origin) {
    try {
      granted = await chrome.permissions.request({ origins: [`${origin}/*`] });
    } catch {
      granted = false;
    }
  }
  await chrome.storage.local.set(next);
  await render();
  say(
    granted
      ? 'Saved.'
      : 'Saved, but the browser has not allowed this extension to reach that address. Nothing can be checked until it does: press Save again and allow it.',
  );
}

async function test() {
  say('Asking Tamga...');
  let reply;
  try {
    reply = await chrome.runtime.sendMessage({ type: MSG_TEST });
  } catch {
    reply = undefined;
  }
  if (reply && reply.ok === true) say('Tamga answered with a verdict. The connection works.');
  else say(reasonText(reply && typeof reply.reason === 'string' ? reply.reason : 'bad_message'));
}

form.addEventListener('submit', save);
document.getElementById('test').addEventListener('click', test);
render();
