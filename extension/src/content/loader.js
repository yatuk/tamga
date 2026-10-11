// The content script the manifest names. A content script cannot be an ES
// module, so this one registers the listeners at document_start, before the
// page's own scripts run, and then imports the rest. The listeners are on
// window in the capture phase: they see a key press or a click before any
// listener of the page.
(() => {
  const hooks = { handle: null };
  for (const type of ['keydown', 'click', 'submit', 'paste']) {
    window.addEventListener(
      type,
      (event) => {
        if (hooks.handle) hooks.handle(event);
      },
      true,
    );
  }
  import(chrome.runtime.getURL('src/content/main.js'))
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
