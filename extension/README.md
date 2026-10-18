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
interface language. Written after that run and run only under `node --test`,
never on a live site: the notice at page load and at a paste when Tamga
cannot be asked, the notice for `STRIP`, and the generic fallback's way of
finding a send button that is in no form and has no English name. Written on
2026-10-18 and likewise run only under `node --test`: the check of
[attached text files](#attached-files) and the [Firefox
build](#firefox). The flows above that were not tried live are now each
run end to end under Node, from the page through the worker's handler to a
fake Tamga (`test/flows.test.js`); the [live-test
checklist](#live-test-checklist) is what is left to confirm on the sites.
Those sites change their markup without notice. Try it
on each site yourself before relying on it, and read
[what it does not cover](../docs/browser-extension.md#what-it-does-not-cover).

The project publishes nothing to the Chrome Web Store or to Edge Add-ons. You
load it unpacked, or pack and sign it yourself and install it by policy:
[docs/browser-extension.md](../docs/browser-extension.md).

Chrome and Edge (version 111 or later) and Firefox (140 or later), Manifest
V3, from the same sources. No dependencies. For Chromium the files in this
directory are the extension; for Firefox, `node tools/package.js firefox`
writes the manifest Firefox needs ([Firefox](#firefox)).

## What it does

| Verdict | On send (Enter or the send button) | On paste |
|---|---|---|
| `PASS` | The message is sent. Nothing is shown | Nothing is shown. The send that follows needs no second check for 30 seconds if the text is unchanged |
| `WARN` | Held. The warning names the kinds of finding; "Send anyway" sends it | The warning is shown |
| `REDACT` | Held. The redacted text Tamga returned is shown; "Use this text" puts it in the box in place of the original. With `autoRedact` it replaces the original at once and says so. Either way the message is not sent: you read it and send it, and it is checked again | The same |
| `STRIP` | Held and not sent. The notice says the policy removes the whole text and names the kinds of finding it does that for. There is no changed text to offer: see below | The same notice is shown |
| `BLOCK` | Held and not sent. The notice names the kinds of finding | The notice is shown |
| No verdict | `failMode: closed` (the default): held, with the reason and "Try again". `failMode: open`: sent, with a notice that it was not checked | The paste is left alone. A quiet notice says why Tamga could not be asked and what happens at the send; it is shown once for a reason, not once for a paste |

"No verdict" is: Tamga not configured, the browser not allowed to reach it,
no answer within the timeout (2.5 seconds unless set), a network error, any
5xx (`tamga_scan_unavailable` and `tamga_scan_busy` included), 401, 403, 429,
a text over the scan API's 256 KB body limit, or an answer that is not a
verdict.

A notice names kinds (`email`, `aws_access_key`) and counts, never a value:
the scan API does not return the matched text and the extension does not
look for it. Nothing is changed or stopped without a notice.

**`STRIP`.** On the proxy path `STRIP` replaces the whole piece of text a
finding is in (a tool result, say) with `[Content removed by Tamga security
policy.]` and forwards the rest of the request. The scan API answers `STRIP`
with the findings and no text (`redacted_text` is for `REDACT` only). For one
text typed by a person there is no rest: stripping it leaves nothing to send.
So the extension does not send it, and says so in those words: "the policy
removes this whole text", then the kinds a rule strips for, then that the
part has to be taken out by hand. It offers no "send anyway" and no changed
text. A `REDACT` that arrives with no text is not sent either and is worded
as that, not as a `STRIP`.

**When Tamga cannot be asked.** The person is told before they try to send:

- When the page loads and the extension has no Tamga address or key, one
  notice says "Tamga is not set up in this browser yet", with "Open
  settings".
- At the first paste whose check fails, one notice says why: the same "not
  set up" notice when the address, the key or the browser's permission is
  missing, "Tamga cannot be reached" when it timed out, could not be reached
  or answered 5xx, and "Tamga could not check this text" for the rest (the
  key refused, too many checks, a text too long).
- The notice holds nothing and steals no focus. The same reason is not said
  twice on one page load; a different reason is. A send that fails says the
  reason in its own notice, and the paste after it does not repeat it.
- Under `failMode: open` the notice says what that means: what is sent from
  the page goes to the site unchecked. Under `closed` it says the message
  will be held at the send.

## Attached files

A file attached to a message is checked before the site has it, when it is
a text file: picked in the site's file dialog (the file input's `input` and
`change` events), dropped on the page, or pasted as a file. The event is
stopped in the capture phase before the page's own listeners, each text file
is read in the content script and sent to the scan API, and only then is the
page given the files again: the file input's events are dispatched once more
(the input still holds the files), or a new `drop` or `paste` event is
dispatched carrying a new `DataTransfer` with the same files, since a drop's
or a paste's own data cannot be read once its event is over.

**Which files are read.** By extension: plain text, Markdown, CSV and TSV,
JSON, YAML, XML, logs, configuration files (`.ini`, `.toml`, `.env`, ...),
SQL, HTML and CSS, and source code (the list is `TEXT_EXTENSIONS` in
`src/content/files.js`); a name with no extension the list knows is read when
the browser calls its type `text/*` or JSON, XML or YAML. Up to 200 KB a
file (the scan API reads 256 KB, and JSON escaping makes the body larger
than the file) and 10 text files at one time. A file is read as UTF-8, with
anything that is not UTF-8 replaced; one that holds NUL characters is not
text, whatever its name.

**What is sent.** One scan per file, with `Attached file: <name>` as the
first line and the file's text after a blank line. The scan API takes one
text and no other field, so that is how the policy gets the name; a finding
in the name is found too. The name is shown and sent without control or
direction-override characters and cut at 120 characters.

| Verdict | What happens to the attach |
|---|---|
| `PASS` | The page is given the files. Nothing is shown |
| `WARN` | Held. The warning names the file and the kinds; "Attach anyway" gives the page the files, "Do not attach" drops them |
| `REDACT` | Not attached. See below |
| `STRIP` | Not attached. The notice says the policy removes the whole text and for what |
| `BLOCK` | Not attached. The notice names the file and the kinds |
| No verdict | `failMode: closed`: held, with "Try again" and "Do not attach". `open`: attached, with a notice that it was not checked. A file too large, more than 10 files, or a file that is not text: not attached under `closed`, attached unchecked under `open`, without "Try again" |

With several files, one that is stopped stops the attach: none of the files
goes, and the files after it are not scanned. A file input that was stopped
is emptied, so the site cannot read the files from it later. An empty text
file costs no scan. A file that was checked and given to the page may be
added again for 30 seconds without a second check, which is also the way
round a browser that would not build the new event: the person is told to
add it again.

**`REDACT` on a file: not attached.** For a typed text the extension offers
the redacted text and the person puts it in the box. For a file there is no
such step: the extension would have to make a new file under the same name
with placeholders in it, which the person has not seen, and hand that to the
site in place of the one they chose. It does not. The file is not attached,
and the notice says that Tamga does not change files, which kinds the policy
changes, and that the person can take those parts out of the file, or paste
its text into the message box, where the redacted text is offered as for
any typed text. `autoRedact` does not change this.

**Not read: PDF, images, office documents, archives, and anything else that
is not text.** The extension cannot get text out of them. Such a file is not
held: it goes to the site as it is, and a quiet notice says, once for a page
load, that it was not checked and which files Tamga reads. A paste that also
carries text (cells copied from a spreadsheet come with a picture of them)
is treated as the text paste it is, with no notice about the picture.

Not covered: a file input the site creates and never puts in the page (its
events do not reach `window`, so the extension does not see them), a file
the site reads by any other way than these events, and a file sent from the
site's own cloud-drive picker.

## Install and configure (one browser, by hand)

1. `chrome://extensions` (or `edge://extensions`), turn on Developer mode,
   "Load unpacked", choose this `extension/` directory. For Firefox, see
   [Firefox](#firefox).
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
In Firefox the same values come from the `3rdparty` section of its
enterprise policy ([docs/browser-extension.md](../docs/browser-extension.md#firefox)).

## Firefox

None of this was run in Firefox. It is written from Mozilla's documentation
and checked under `node --test` (`test/package.test.js`).

```bash
cd extension
node tools/package.js            # both; or: node tools/package.js firefox
```

writes `dist/firefox/` (the extension as a directory) and
`dist/tamga-firefox-<version>.zip`, and the same for `chromium`. The sources
are the same files; only the manifest differs, and it is made from
`manifest.json` by `tools/package.js`:

- `background.scripts` in place of `background.service_worker`: Firefox
  has no extension service workers and runs the same module as an event
  page. Nothing in the worker depends on which it is.
- `browser_specific_settings.gecko`: the add-on ID (`chat-sites@tamgaproxy.com`
  unless `--gecko-id` gives another), `strict_min_version` 140 (the first
  release, and the current ESR, that has the data-consent key), and
  `data_collection_permissions` with `websiteContent` required: the text of
  what is pasted, sent or attached goes to your own Tamga, which is off the
  device.
- No `minimum_chrome_version` and no `storage.managed_schema`: Firefox
  reads managed values from policy without a schema.
- `web_accessible_resources` names `src/content/adapters/*` as well, so
  that the content script's imports do not depend on a wildcard reaching
  into a subdirectory.

The code calls the browser through `ext` (`src/shared/api.js`), which is
`browser` in Firefox and `chrome` in Chrome and Edge; both return promises in
Manifest V3. The settings page asks for the Tamga origin before it waits for
anything, because Firefox takes `permissions.request` as the person's own
only while the click's handler has not yet awaited.

To try it on one machine: `about:debugging`, "This Firefox", "Load Temporary
Add-on...", choose `dist/firefox/manifest.json`. It stays until Firefox is
closed. Then open its options (`about:addons`, the extension, Preferences)
and fill them in as above. In Firefox a Manifest V3 extension's access to
sites can be withdrawn by the person: check under `about:addons`, the
extension, Permissions, that the three sites are allowed, or the content
script does not run there at all. To install it for an organisation (signing
and enterprise policy): [docs/browser-extension.md](../docs/browser-extension.md#firefox).

Known differences that were not tried: the `Origin` the event page sends
(expected `moz-extension://<uuid>`, which Tamga lets through as it does
`chrome-extension://`), whether a page accepts a `drop` or `paste` event made
by a content script in Firefox (its content scripts run behind Xray
wrappers), and whether a host permission is granted at installation by
policy.

## How it is put together

```
manifest.json            permissions: storage; hosts: the three sites
managed_schema.json      the settings an administrator can set by policy
options/                 the settings page
src/shared/              message shapes, the list of sites, browser.* or chrome.*
src/background/          the service worker: settings, the key, the one fetch
src/content/loader.js    the content script: listens from document_start
src/content/controller.js   what happens on paste, send and attach
src/content/decide.js       from a reply to an outcome
src/content/files.js        which attached files are read, and how
src/content/ui.js           the notice, in a closed shadow root
src/content/adapters/       one module per site, and the generic fallback
tools/package.js         the package for each browser, and the Firefox manifest
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
- An attached file is read in the content script, and only its text goes
  to the worker. The file is not stored and not changed.
- The text is not stored. A text that got PASS is held in the content
  script's memory for at most 30 seconds, so that a paste followed by Enter
  is one scan and not two; a text that got another verdict for 10 seconds,
  so that pressing Enter again shows the notice without a new scan.
- One tab can ask for at most 30 scans a minute, so that a page scripting
  pastes cannot spend the key's limit at Tamga.

## Site adapters

An adapter answers: where is the message box, what text is in it, how is the
text replaced, which element sends, and which file inputs take attachments.
Each site's selectors are in one file under `src/content/adapters/`.
Selectors are tried in order.

The file-input selectors were added on 2026-10-18 and are guesses: none was
compared with a live page, and the file inputs in the fixtures are assumed.
That matters less than for the send button, because a file input the site's
own selectors miss is still watched: the generic fallback takes every
`input[type="file"]` on the page, since on these sites a file is picked to be
sent.

**The selectors were compared with the live pages once, on 2026-10-11** (Chrome, signed in, Turkish interface), by reading the page without the extension loaded: which selector finds the message box, and which finds the send button once the box holds text. Nothing was sent in that comparison; what was then run with the extension loaded, and what was not, is in the status paragraph at the top. Before that comparison every selector here was a guess: the message box was found on all three sites, the send button on one of three. The send button's label is translated with the interface, so a selector that names it in English fails for most people; the ones added from the live page do not depend on the label.

Each adapter has a fixture under `test/fixtures/` that reproduces the structure
seen on that day; the tests prove the adapter matches its fixture, not the
site. The ChatGPT and Gemini fixtures also hold the buttons that must not be
taken for the send button (attach and voice; upload, dictation and the model
selector). Where those sit in the tree and what they are called is assumed,
not copied from the sites.

On gemini.google.com the box sits in no form and the send button has no name
but its translated label. The generic fallback used to find the box and not
the button there. It now finds the button of the fixture, by the Turkish
label and by the class of the element that wraps it (see "the generic
fallback's send button" below), so with Gemini's own selectors broken a send
by button is held in the tests as a send by Enter is. That was not tried on
gemini.google.com. With an interface in a language outside the short list
below, and a wrapper that no longer carries `send` in its class, the button
is found only once the fallback has seen the box empty and then with text;
until then a send by Enter is held and a send by button is not.

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
| claude.ai | file input | `input[data-testid="file-upload"]` | a guess, not compared with the live page (2026-10-18) |
| claude.ai | file input | `fieldset input[type="file"]` | a guess, not compared with the live page (2026-10-18) |
| chatgpt.com | file input | `form input[type="file"]` | a guess, not compared with the live page (2026-10-18) |
| gemini.google.com | file input | `.upload-button-container input[type="file"]` | a guess, not compared with the live page (2026-10-18) |
| gemini.google.com | file input | `input[type="file"][name="Filedata"]` | a guess, not compared with the live page (2026-10-18) |
| generic fallback | file input | `input[type="file"]` | every file input on the page |

**The generic fallback's send button, where no selector above matches.** This
part is not a selector and so is not in the table. It applies only when the
site's own selectors find no send button on the page, and only to a `button`
that comes after the message box in the page, within a few levels of it (up
to six, and no further than a `form` or `main`). Such a button is taken for
the send button on one of two grounds:

1. **It appeared with the text.** Every two seconds, while the box is empty,
   the fallback notes which of those buttons can be pressed. A button that
   can be pressed once the box holds text and could not be before (it was
   absent, or disabled) is the send button, if it is the only one. Two such
   buttons are not guessed between. The answer is kept for the page.
2. **It is named "send" in one of nine languages.** Its `aria-label`,
   `title`, `data-testid` or `class`, or the `class` or `data-testid` of an
   element that wraps that button alone, holds one of: `send`, `gönder`,
   `enviar`, `envoyer`, `senden`, `invia`, `отправить`, `发送`, `送信`. That is
   a list, in `adapters/generic.js`, and it is short on purpose. An interface
   in any other language is covered by the first ground or not at all.

Not used: the button's type (the upload and dictation buttons on
gemini.google.com are `type="submit"` too) and its icon. Both grounds are
guesses. The first is wrong on a page where some other single button appears
only when there is text; the extension would then check the text on that
button's click and, after a PASS, click it.

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
  (anything other than `keydown`, `click` and `submit`, and for files
  `input`, `change`, `drop` and `paste`) is not stopped.
- **A page takes a dispatched attach as a real one.** After a check the
  page is given a file input's `input` and `change` events again, or a new
  `drop` or `paste` event with the same files. A site that ignores events a
  script made, or that reads the files some other way, will not take the
  file; the person then adds it again, which goes through without a second
  check for 30 seconds.

When a site's selectors match nothing, the generic fallback is used: the
textarea or contenteditable element the key press happened in, or the one in
the send button's form, or the one the send button stands after. A badge in
the corner says the fallback is in use.
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
cover: each verdict, fail-closed and fail-open, the timeout, the notice when
Tamga cannot be asked (at page load, at a paste, once for a reason), the
message checks in both directions, each adapter against its fixture, the
buttons the fallback must not take for a send button, that no code
path fetches an origin other than the configured one, and that the key is in
no reply to a content script. `test/files.test.js` covers attached files for
every verdict and every way in (file input, drop, paste);
`test/flows.test.js` runs the flows the live run did not try (a clean text
sent on, a held send resumed, a paste, Tamga down or busy, a file) from the
page through the worker's real handler to a fake Tamga; `test/package.test.js`
covers the packages and the Firefox manifest.

If `dashboard/node_modules` is installed, the same tests run on jsdom, as a
check that the small DOM is not hiding a difference:

```bash
TAMGA_EXT_DOM=jsdom node --test
```

The attached-file tests are skipped there: jsdom cannot put files in a file
input or build a `DataTransfer`.

To look at a fixture in a browser, serve this directory
(`python -m http.server` in `extension/`) and open
`http://localhost:8000/test/fixtures/claude.html`. The extension does not
run there: its content script matches the three sites only.

## Live-test checklist

For the owner, on each of claude.ai, chatgpt.com and gemini.google.com,
with the extension loaded unpacked against a Tamga you can watch the log of.
The Node tests already run each of these; the live run confirms that the
site behaves as the fixtures assume. Note the browser, its version, the
interface language and the date for each line, and change the status
paragraph at the top to match.

Text:

1. A clean text, sent by Enter: it is sent once, nothing is shown, Tamga
   logs one `scan api call` with `PASS`.
2. The same by the send button.
3. A held send resumed: stop Tamga's answer for a second or two (a slow
   proxy, or `timeoutMs` raised and Tamga paused), send, see "Tamga is
   checking this message", let it answer: the message is sent and the
   notice goes.
4. A paste of a clean text, then Enter: one scan in Tamga's log, not two.
5. A paste of a text the policy blocks: the notice comes at once; Enter is
   then held without a second scan.
6. Tamga down (stop the proxy) under `failMode: closed`: a paste says
   "Tamga cannot be reached" once, quietly; Enter is held with "Try again";
   start Tamga, press "Try again": the message is sent.
7. Tamga down under `failMode: open`: the message is sent and the notice
   says it was not checked.

Files (each through the attach button, by drag and drop, and by pasting a
file copied in the file manager):

8. A clean `.txt` or `.csv`: it appears in the site's attachment area as
   without the extension; Tamga's log has one scan for it.
9. A `.csv` with a value the policy blocks (the test TCKN `10000000146`):
   it does not appear, the notice names the file and the kind; through the
   attach button, picking the same file again opens the dialog fresh.
10. A file the policy redacts: not attached, and the notice says so.
11. A PDF or an image: attached as before, and one quiet notice says it was
    not checked; a second one says nothing.
12. Tamga down under `failMode: closed`: a text file is held with "Try
    again"; start Tamga, press it: the file appears.
13. If a checked file does not appear: the notice says to add it again, and
    adding it again goes through. Note which way in (button, drop, paste)
    and which site, since that means the site ignores the event the
    extension dispatched.

Firefox (`node tools/package.js firefox`, then `about:debugging`): lines 1,
6, 8 and 9 on one site, and the options page's Save asking for the Tamga
origin.

## Licence

AGPL-3.0-only. `LICENSE` is a copy of the repository's; a test keeps the two
identical.
