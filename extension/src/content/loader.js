// The content script the manifest names. A content script cannot be an ES
// module, so this one registers the listeners at document_start, before the
// page's own scripts run, and then imports the rest. The listeners are on
// window in the capture phase: they see a key press or a click before any
// listener of the page.
(() => {
  // browser.* in Firefox, chrome.* in Chrome and Edge (src/shared/api.js).
  const ext = globalThis.browser ?? globalThis.chrome;
  const hooks = { handle: null };
  // input and change for file inputs, drop and paste for files: an attach
  // is held as a send is.
  for (const type of ['keydown', 'click', 'submit', 'paste', 'input', 'change', 'drop']) {
    window.addEventListener(
      type,
      (event) => {
        if (hooks.handle) hooks.handle(event);
      },
      true,
    );
  }
  import(ext.runtime.getURL('src/content/main.js'))
    .then((main) => main.start(hooks))
    .catch(() => {
      // The extension is on the page and does nothing: say so.
      const note = document.createElement('div');
      note.textContent = 'Tamga failed to start on this page. Nothing typed here is checked.';
      note.style.cssText =
        'position:fixed;right:16px;bottom:16px;z-index:2147483647;background:#fff;color:#161616;' +
        'border:1px solid #161616;border-left:4px solid #b82a18;padding:6px 10px;font:12px system-ui,sans-serif';
      (document.documentElement || document.body).appendChild(note);
    });
})();
