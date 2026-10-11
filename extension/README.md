# Tamga for chat sites (browser extension, MVP)

Tamga's proxy sees the requests applications send to it. It does not see what
someone types into claude.ai, chatgpt.com or gemini.google.com in a browser.
This extension asks your own Tamga, at the moment of paste or send on those
three sites, what the policy says about the text
([`POST /api/v1/scan`](../docs/operations.md#checking-a-text-without-sending-it)),
and acts on the answer before the text reaches the site.

**Status: an MVP, run once on the live sites.** On 2026-10-11 the unpacked
extension was loaded in one Chromium browser (signed in, Turkish interface)
against a local Tamga, and on claude.ai, chatgpt.com and gemini.google.com
each of these was seen to work: a send by Enter and a send by the button
held and stopped for a text the policy blocks; the redacted text offered for
a text the policy redacts and put in the box, with nothing sent; and, before
a key was stored, the send held with "Tamga is not set up" (seen on claude.ai only). Not tried on the
sites: a clean text passing and being sent on, a held send resumed after a
check, a paste, Tamga being down, and any other browser, account type or
interface language. Those sites change their markup without notice. Try it
on each site yourself before relying on it, and read
[what it does not cover](../docs/browser-extension.md#what-it-does-not-cover).

The project publishes nothing to the Chrome Web Store or to Edge Add-ons. You
load it unpacked, or pack and sign it yourself and install it by policy:
[docs/browser-extension.md](../docs/browser-extension.md).

Chromium only (Chrome and Edge, version 111 or later), Manifest V3. No build
step and no dependencies: the files in this directory are the extension.

## What it does

| Verdict | On send (Enter or the send button) | On paste |
|---|---|---|
| `PASS` | The message is sent. Nothing is shown | Nothing is shown. The send that follows needs no second check for 30 seconds if the text is unchanged |
| `WARN` | Held. The warning names the kinds of finding; "Send anyway" sends it | The warning is shown |
| `REDACT` | Held. The redacted text Tamga returned is shown; "Use this text" puts it in the box in place of the original. With `autoRedact` it replaces the original at once and says so. Either way the message is not sent: you read it and send it, and it is checked again | The same |
| `STRIP` | Held and not sent. The scan API returns no text for STRIP, so there is nothing to offer | The notice is shown |
| `BLOCK` | Held and not sent. The notice names the kinds of finding | The notice is shown |
| No verdict | `failMode: closed` (the default): held, with the reason and "Try again". `failMode: open`: sent, with a notice that it was not checked | Nothing is shown; the send decides |

"No verdict" is: Tamga not configured, the browser not allowed to reach it,
no answer within the timeout (2.5 seconds unless set), a network error, any
5xx (`tamga_scan_unavailable` and `tamga_scan_busy` included), 401, 403, 429,
a text over the scan API's 256 KB body limit, or an answer that is not a
verdict.

A notice names kinds (`email`, `aws_access_key`) and counts, never a value:
the scan API does not return the matched text and the extension does not
look for it. Nothing is changed or stopped without a notice.

## Install and configure (one browser, by hand)

1. `chrome://extensions` (or `edge://extensions`), turn on Developer mode,
   "Load unpacked", choose this `extension/` directory.
2. Open the extension's options. Enter the Tamga URL (`https://...`; `http`
   is accepted for `localhost` only) and a Tamga key of the `proxy` scope.
   On Save the browser asks whether the extension may reach that one origin;
   allow it.
3. "Test the connection" sends the words "connection test" to
   `/api/v1/scan` and says what came back.

Settings: `tamgaUrl`, `tamgaKey`, `failMode` (`closed` or `open`),
`autoRedact` (`false`), `sites` (all three), `timeoutMs` (2500, kept between
500 and 10000; policy only). A value set by policy
(`chrome.storage.managed`, described by `managed_schema.json`) wins over one
typed into the options page, field by field, and is shown there as locked.

## How it is put together

```
manifest.json            permissions: storage; hosts: the three sites
managed_schema.json      the settings an administrator can set by policy
options/                 the settings page
src/shared/              message shapes and the list of sites
src/background/          the service worker: settings, the key, the one fetch
src/content/loader.js    the content script: listens from document_start
src/content/controller.js   what happens on paste and send
src/content/decide.js       from a reply to an outcome
src/content/ui.js           the notice, in a closed shadow root
src/content/adapters/       one module per site, and the generic fallback
test/                    node --test; fixtures under test/fixtures/
```

- The key is read by the service worker and by the options page. Content
  scripts never read storage (the worker also closes the storage areas to
  them where the browser supports it), and no reply to a content script
  holds the key or the Tamga URL.
- The only network request is the worker's `POST {tamgaUrl}/api/v1/scan`,
  with `credentials: omit`, `redirect: error` and no referrer. There are no
  analytics, no remote code and no other request.
- The worker answers only its own content scripts in the top frame of a tab
  on one of the three sites, and its own options page. A message is rebuilt
  from its allowed fields; anything else is refused. A content script
  accepts a reply only in the expected shape.
- Page content and API text are set as text nodes. There is no `innerHTML`.
- The text is not stored. A text that got PASS is held in the content
  script's memory for at most 30 seconds, so that a paste followed by Enter
  is one scan and not two; a text that got another verdict for 10 seconds,
  so that pressing Enter again shows the notice without a new scan.
- One tab can ask for at most 30 scans a minute, so that a page scripting
  pastes cannot spend the key's limit at Tamga.

## Site adapters

An adapter answers: where is the message box, what text is in it, how is the
text replaced, which element sends. Each site's selectors are in one file
under `src/content/adapters/`. Selectors are tried in order.

**The selectors were compared with the live pages once, on 2026-10-11** (Chrome, signed in, Turkish interface), by reading the page without the extension loaded: which selector finds the message box, and which finds the send button once the box holds text. Nothing was sent. **The extension itself has still not been run on any of the three sites**, so whether a held send, a replaced text or a resumed send works there is not known. Before that comparison every selector here was a guess: the message box was found on all three sites, the send button on one of three. The send button's label is translated with the interface, so a selector that names it in English fails for most people; the ones added from the live page do not depend on the label.

Each adapter has a fixture under `test/fixtures/` that reproduces the structure
seen on that day; the tests prove the adapter matches its fixture, not the
site. On gemini.google.com the box sits in no form and the send button has
no name but its translated label, so the generic fallback finds the box and
not the button there: if Gemini's own selectors stop matching, a send by
Enter is still held and a send by button is not.

| Site | Role | Selector | Status (2026-10-11) |
|---|---|---|---|
| claude.ai | message box | `div[data-testid="chat-input"][contenteditable="true"]` | added from the live page |
| claude.ai | message box | `div.ProseMirror[contenteditable="true"]` | seen, 1 match |
| claude.ai | message box | `fieldset div[contenteditable="true"]` | seen, 1 match |
| claude.ai | message box | `div[contenteditable="true"][role="textbox"]` | seen, 1 match |
| claude.ai | send | `button[data-testid="chat-input-send"]` | added from the live page |
| claude.ai | send | `button[aria-label="Send message"]` | seen, 1 match (English label on a Turkish account; a translated label would miss) |
| claude.ai | send | `button[aria-label="Send Message"]` | no match |
| claude.ai | send | `fieldset button[type="submit"]` | no match |
| chatgpt.com | message box | `div#prompt-textarea[contenteditable="true"]` | no match (the box has no id) |
| chatgpt.com | message box | `textarea#prompt-textarea` | no match |
| chatgpt.com | message box | `form div.ProseMirror[contenteditable="true"]` | seen, 1 match |
| chatgpt.com | send | `button[data-testid="send-button"]` | no match |
| chatgpt.com | send | `button#composer-submit-button` | no match |
| chatgpt.com | send | `button[aria-label="Send prompt"]` | no match (label is translated: "Gönder") |
| chatgpt.com | send | `button[aria-label="Send message"]` | no match |
| chatgpt.com | send | `form button[type="submit"]` | added from the live page |
| gemini.google.com | message box | `rich-textarea div.ql-editor[contenteditable="true"]` | seen, 1 match |
| gemini.google.com | message box | `div.ql-editor[contenteditable="true"]` | seen, 1 match |
| gemini.google.com | message box | `div[contenteditable="true"][role="textbox"]` | seen, 1 match |
| gemini.google.com | send | `.send-button button` | added from the live page |
| gemini.google.com | send | `.send-button-container button` | added from the live page |
| gemini.google.com | send | `button.send-button` | no match (the class is on the wrapper) |
| gemini.google.com | send | `button[aria-label="Send message"]` | no match (label is translated: "Mesaj gönder") |
| gemini.google.com | send | `button[aria-label="Send"]` | no match |
| generic fallback | message box | `main form textarea` | a guess by construction |
| generic fallback | message box | `main form [contenteditable="true"]` | a guess by construction |
| generic fallback | message box | `form textarea` | a guess by construction |
| generic fallback | message box | `form [contenteditable="true"]` | a guess by construction |
| generic fallback | message box | `main textarea` | a guess by construction |
| generic fallback | message box | `main [contenteditable="true"]` | a guess by construction |
| generic fallback | message box | `textarea` | a guess by construction |
| generic fallback | message box | `[contenteditable="true"]` | a guess by construction |
| generic fallback | send | `button[aria-label*="Send"]` | a guess by construction |
| generic fallback | send | `button[aria-label*="send"]` | a guess by construction |
| generic fallback | send | `button[data-testid*="send"]` | a guess by construction |
| generic fallback | send | `button[type="submit"]` | only in a form that holds a message box |
| generic fallback | send | `input[type="submit"]` | only in a form that holds a message box |

Also assumed and equally unverified:

- **Enter sends, Shift+Enter does not.** Enter without Shift or Alt in a
  message box is treated as a send. Where a site uses Enter for something
  else (choosing from a menu that is open over the box, a new line on a
  narrow layout) the extension will check the text and then press the send
  button, which is not what the person meant.
- **A scripted click on the send button sends.** After a PASS the extension
  clicks the send button itself (or, with no button found, dispatches an
  Enter key press). If the site ignores that, the text is still in the box
  0.8 seconds later and the extension says "press Enter or the send button
  again"; that second, real key press goes through without a new scan.
- **`insertText` replaces the text.** A redacted text is put in a rich-text
  box by selecting its contents and `document.execCommand('insertText')`,
  falling back to setting the text and firing `input`. The extension reads
  the box back afterwards; if it does not hold the redacted text it says so
  and shows the text to copy by hand. The original is not sent either way.
- **The listeners run first.** They are registered on `window` in the capture
  phase at `document_start`, before the page's scripts, and stop the event
  there. A site that sends from an event the extension does not listen for
  (anything other than `keydown`, `click` and `submit`) is not stopped.

When a site's selectors match nothing, the generic fallback is used: the
textarea or contenteditable element the key press happened in, or the one in
the send button's form. A badge in the corner says the fallback is in use.
When there is no such element on the page at all, the badge says "Tamga could
not attach to this page", and a click on anything that looks like a send
button is held under `failMode: closed` (let through with a notice under
`open`). A content script cannot do more than that: it cannot stop a page
from sending text it never saw being typed.

### When a site changes

Open the site, find the message box and the send button in the developer
tools, put a selector that matches at the top of the list in the site's
module, update the fixture to the structure you saw, and change the row in
the table above to "verified" with the date. `node --test` fails if the
table and the modules disagree.

## Tests

```bash
cd extension
node --test
```

Node 20 or later, nothing to install. The tests use a small DOM written for
them (`test/helpers/dom.js`) and fakes for `chrome.*` and `fetch`. They
cover: each verdict, fail-closed and fail-open, the timeout, the message
checks in both directions, each adapter against its fixture, that no code
path fetches an origin other than the configured one, and that the key is in
no reply to a content script.

If `dashboard/node_modules` is installed, the same tests run on jsdom, as a
check that the small DOM is not hiding a difference:

```bash
TAMGA_EXT_DOM=jsdom node --test
```

To look at a fixture in a browser, serve this directory
(`python -m http.server` in `extension/`) and open
`http://localhost:8000/test/fixtures/claude.html`. The extension does not
run there: its content script matches the three sites only.

## Licence

AGPL-3.0-only. `LICENSE` is a copy of the repository's; a test keeps the two
identical.
