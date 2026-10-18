// What every adapter is built from. An adapter answers five questions about
// a page: where is the message box, what text is in it, how is that text
// replaced, which element sends it, and which file inputs take attachments.
//
//   id                      'claude' | 'chatgpt' | 'gemini' | 'generic'
//   selectors               { composer: [...], sendButton: [...], fileInput: [...] }
//   findComposer(doc, near) the message box, or null
//   readText(el)            its text
//   replaceText(el, text)   true when the box now holds `text`
//   matchSend(target)       the send button `target` is in, or null
//   findSendButton(doc)     the send button, or null
//   matchFileInput(el)      true when `el` is one of the site's file inputs
//   findFileInputs(doc)     the site's file inputs on the page
//
// A site module holds selectors and nothing else, so that a site changing
// its markup is a change to one short list.

const BLOCKS = new Set(['P', 'DIV', 'LI', 'PRE', 'BLOCKQUOTE', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'TR']);

function isEditableAttr(el) {
  const v = el.getAttribute?.('contenteditable');
  return v === '' || v === 'true' || v === 'plaintext-only';
}

/** The textarea or the outermost contenteditable element `node` is in. */
export function editableRoot(node) {
  let found = null;
  for (let el = node; el; el = el.parentNode) {
    if (el.nodeType !== 1) continue;
    if (el.tagName === 'TEXTAREA') return el;
    if (isEditableAttr(el)) found = el;
  }
  return found;
}

function collect(node, out) {
  for (const child of node.childNodes) {
    if (child.nodeType === 3) {
      out.push(child.nodeValue);
    } else if (child.nodeType === 1) {
      if (child.tagName === 'BR') {
        out.push('\n');
        continue;
      }
      const block = BLOCKS.has(child.tagName);
      if (block && out.length > 0 && !out[out.length - 1].endsWith('\n')) out.push('\n');
      collect(child, out);
      if (block && out.length > 0 && !out[out.length - 1].endsWith('\n')) out.push('\n');
    }
  }
}

/** The text of a textarea or of a contenteditable element, a line per block. */
export function readText(el) {
  if (!el) return '';
  if (el.tagName === 'TEXTAREA') return String(el.value ?? '');
  const out = [];
  collect(el, out);
  return out.join('').replace(/ /g, ' ').replace(/\n+$/, '');
}

const squash = (s) => s.replace(/\s+/g, ' ').trim();

function fireInput(el, text) {
  const win = el.ownerDocument?.defaultView;
  if (!win) return;
  let event;
  try {
    event = new win.InputEvent('input', { bubbles: true, inputType: 'insertText', data: text });
  } catch {
    event = new win.Event('input', { bubbles: true });
  }
  el.dispatchEvent(event);
}

/**
 * Puts `text` in the box in place of what is there. Returns true only when
 * the box reads back as `text` (white space apart), so a caller never
 * believes a replacement that the page's editor undid or ignored.
 */
export function replaceText(el, text) {
  if (!el) return false;
  const doc = el.ownerDocument;
  const win = doc?.defaultView;
  if (el.tagName === 'TEXTAREA') {
    // The prototype's setter, so that a framework watching the property
    // (React does) sees the change.
    const setter = win?.HTMLTextAreaElement
      ? Object.getOwnPropertyDescriptor(win.HTMLTextAreaElement.prototype, 'value')?.set
      : null;
    if (setter) setter.call(el, text);
    else el.value = text;
    fireInput(el, text);
    return squash(readText(el)) === squash(text);
  }

  // A rich-text editor keeps its own model of the document. insertText over
  // a selection of everything goes through the editor's own input handling.
  let inserted = false;
  try {
    el.focus?.();
    const selection = doc.getSelection?.();
    if (selection && doc.createRange) {
      const range = doc.createRange();
      range.selectNodeContents(el);
      selection.removeAllRanges();
      selection.addRange(range);
      inserted = doc.execCommand?.('insertText', false, text) === true;
    }
  } catch {
    inserted = false;
  }
  if (!inserted || squash(readText(el)) !== squash(text)) {
    el.textContent = text;
    fireInput(el, text);
  }
  return squash(readText(el)) === squash(text);
}

function first(doc, selectors) {
  for (const selector of selectors) {
    const el = doc.querySelector(selector);
    if (el) return el;
  }
  return null;
}

function closest(target, selectors) {
  if (!target || typeof target.closest !== 'function') return null;
  for (const selector of selectors) {
    const el = target.closest(selector);
    if (el) return el;
  }
  return null;
}

function isFileInput(el) {
  return Boolean(el) && el.nodeType === 1 && el.tagName === 'INPUT' && String(el.type).toLowerCase() === 'file';
}

/** An adapter from its selector lists, tried in order. */
export function makeAdapter({ id, composer, sendButton, fileInput = [] }) {
  return Object.freeze({
    id,
    selectors: Object.freeze({
      composer: Object.freeze([...composer]),
      sendButton: Object.freeze([...sendButton]),
      fileInput: Object.freeze([...fileInput]),
    }),
    findComposer: (doc) => first(doc, composer),
    readText,
    replaceText,
    matchSend: (target) => closest(target, sendButton),
    findSendButton: (doc) => first(doc, sendButton),
    matchFileInput: (el) => isFileInput(el) && fileInput.some((selector) => el.matches(selector)),
    findFileInputs: (doc) => [...new Set(fileInput.flatMap((selector) => [...doc.querySelectorAll(selector)]))].filter(isFileInput),
  });
}

export { closest, first, isFileInput };
