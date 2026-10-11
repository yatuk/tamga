// The notice the person sees. It lives in a closed shadow root on an element
// the extension adds to the page, so the page's styles do not reach it and
// the page's scripts cannot read it through the DOM. Every string is set as
// text; nothing from the page or from the API is parsed as markup.

const CSS = `
:host { all: initial; }
.wrap { position: fixed; right: 16px; bottom: 16px; z-index: 2147483647; width: 360px; max-width: calc(100vw - 32px);
  display: flex; flex-direction: column; gap: 8px; align-items: flex-end;
  font: 14px/1.45 system-ui, -apple-system, "Segoe UI", sans-serif; color: #161616; }
.notice { box-sizing: border-box; width: 100%; background: #fff; border: 1px solid #161616; border-left-width: 4px; padding: 12px 14px; }
.notice.stop { border-left-color: #b82a18; }
.notice.change { border-left-color: #7a5000; }
.title { font-weight: 600; margin: 0 0 4px; }
.body { margin: 0; }
.detail { margin: 8px 0 0; padding: 8px; max-height: 160px; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere;
  background: #f4f4f4; border: 1px solid #c6c6c6; font: 12px/1.45 ui-monospace, Consolas, monospace; user-select: text; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
button { font: inherit; padding: 5px 12px; border: 1px solid #161616; border-radius: 0; background: #fff; color: #161616; cursor: pointer; }
button.primary { background: #161616; color: #fff; }
button:focus-visible { outline: 2px solid #161616; outline-offset: 2px; }
.badge { background: #fff; border: 1px solid #161616; padding: 4px 8px; font-size: 12px; }
.badge.stop { border-left: 4px solid #b82a18; }
[hidden] { display: none; }
`;

function el(doc, tag, className, text) {
  const node = doc.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

/**
 * Builds the notice area. Returns
 *   show(notice, onAction)  notice: { tone, title, body, detail?, actions: [{ id, label }] }
 *   hide()
 *   badge(text | null, tone?)
 */
export function createUI(doc) {
  const host = doc.createElement('div');
  host.setAttribute('data-tamga', '');
  const root = typeof host.attachShadow === 'function' ? host.attachShadow({ mode: 'closed' }) : host;

  // A constructed style sheet is not subject to the page's style-src; a
  // <style> element is the fallback where constructed sheets are missing.
  const win = doc.defaultView;
  let styled = false;
  try {
    if (root !== host && win?.CSSStyleSheet && 'adoptedStyleSheets' in root) {
      const sheet = new win.CSSStyleSheet();
      sheet.replaceSync(CSS);
      root.adoptedStyleSheets = [sheet];
      styled = true;
    }
  } catch {
    styled = false;
  }
  if (!styled) root.appendChild(el(doc, 'style', '', CSS));

  const wrap = el(doc, 'div', 'wrap');
  const notice = el(doc, 'div', 'notice');
  notice.setAttribute('role', 'alert');
  notice.hidden = true;
  const badgeEl = el(doc, 'div', 'badge');
  badgeEl.setAttribute('role', 'status');
  badgeEl.hidden = true;
  wrap.appendChild(notice);
  wrap.appendChild(badgeEl);
  root.appendChild(wrap);

  function mount() {
    const parent = doc.documentElement || doc.body;
    if (parent && host.parentNode !== parent) parent.appendChild(host);
  }

  function clear(node) {
    while (node.firstChild) node.removeChild(node.firstChild);
  }

  return {
    show(spec, onAction) {
      mount();
      clear(notice);
      notice.className = `notice ${spec.tone === 'stop' || spec.tone === 'change' ? spec.tone : 'note'}`;
      notice.appendChild(el(doc, 'p', 'title', spec.title));
      if (spec.body) notice.appendChild(el(doc, 'p', 'body', spec.body));
      if (typeof spec.detail === 'string') notice.appendChild(el(doc, 'div', 'detail', spec.detail));
      if (spec.actions.length > 0) {
        const row = el(doc, 'div', 'actions');
        spec.actions.forEach((action, i) => {
          const button = el(doc, 'button', i === 0 ? 'primary' : '', action.label);
          button.type = 'button';
          button.addEventListener('click', (event) => {
            // Only a click the person made counts: the page cannot press
            // "Send anyway" with a script.
            if (event.isTrusted === false) return;
            onAction(action.id);
          });
          row.appendChild(button);
        });
        notice.appendChild(row);
      }
      notice.hidden = false;
    },
    hide() {
      notice.hidden = true;
      clear(notice);
    },
    badge(text, tone) {
      if (!text) {
        badgeEl.hidden = true;
        return;
      }
      mount();
      badgeEl.className = tone === 'stop' ? 'badge stop' : 'badge';
      badgeEl.textContent = text;
      badgeEl.hidden = false;
    },
  };
}
