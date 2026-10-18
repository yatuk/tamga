// The extension API: browser.* in Firefox, chrome.* in Chrome and Edge.
// In Manifest V3 both return promises from every call the extension makes,
// so the rest of the code does not tell them apart. loader.js, which cannot
// import a module, picks the same way.

export const ext = globalThis.browser ?? globalThis.chrome;
