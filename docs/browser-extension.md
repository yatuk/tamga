# Browser extension for chat sites

Tamga sees the requests that are sent to it. Text typed into claude.ai,
chatgpt.com or gemini.google.com goes from the browser to that site and never
reaches the proxy. The extension in [`extension/`](../extension/README.md)
is for that gap: when someone pastes into the message box or sends on one of
those three sites, it asks your own Tamga what the policy says about the text
([`POST /api/v1/scan`](operations.md#checking-a-text-without-sending-it)) and
acts on the answer before the site receives anything.

> **Status: an MVP, run once on the live sites.** On 2026-10-11 it was loaded
> unpacked in one browser and a blocked send and a redacted text were seen
> to work on claude.ai, chatgpt.com and gemini.google.com.
> [`extension/README.md`](../extension/README.md) is the record of that run:
> what was seen, what was not tried (a paste, Tamga being down, a clean text
> sent on, any other browser or interface language), which selector matched
> the live page, and which parts were written afterwards and have only been
> run by the Node tests. Check it on each site yourself before you depend on
> it. Those sites change their pages without notice, and when they do, an
> adapter stops matching until someone updates it. The check of attached
> text files and the Firefox build were written on 2026-10-18 and have been
> run only by the Node tests; the README ends with a checklist for the live
> run.

The project publishes nothing to the Chrome Web Store or to Edge Add-ons.
There is no listing to install from. You load the directory unpacked, or you
pack and sign it yourself and install it by policy.

## What it does

| Verdict from Tamga | What the person sees |
|---|---|
| `PASS` | Nothing. The message is sent |
| `WARN` | The warning, with the kinds of finding. "Send anyway" sends it |
| `REDACT` | The text Tamga returned with placeholders in it. "Use this text" puts it in the box in place of the original, or it is put there at once when `autoRedact` is on. The person then reads it and sends it; that send is checked like any other |
| `STRIP` | The message is not sent. The notice says the policy removes the whole text and names the kinds of finding it does that for. The scan API returns no replacement text for `STRIP`: on the proxy path the whole piece of text is replaced by a notice, and for one typed message that leaves nothing to send |
| `BLOCK` | The message is not sent. The notice names the kinds of finding |
| No verdict | `failMode: closed`, the default: the message is held, with the reason and "Try again". `failMode: open`: it is sent and a notice says it was not checked |

A notice names kinds and counts (`email (2), iban`), never a value. Nothing is
stopped or changed without a notice.

A paste is not held: the text lands in the box and the whole box is checked
straight away, so that a problem is shown before the person goes on writing.
A send is always held until the text that is in the box at that moment has a
verdict.

An attached text file (plain text, Markdown, CSV, JSON, YAML, XML, logs,
source code; up to 200 KB, up to 10 at a time) is held before the site has
it, whether it is picked in the site's file dialog, dropped on the page or
pasted as a file. Each is read in the browser and sent to the scan API with
its name as the first line; the site gets the files only after the
verdict. `PASS` hands them on, `WARN` asks, and `BLOCK`, `STRIP` and
`REDACT` keep the whole attach from the site. A file is not rewritten for
`REDACT`: the extension does not hand the site a changed copy of a person's
file that they have not seen, so the notice says to take those parts out or
paste the text into the box, where the redacted text is offered. With no
verdict the fail mode decides, as for a send. PDF files, images, office
documents and other files that are not text are not read; they go to the
site as they are, and the person is told so once for a page load
([details](../extension/README.md#attached-files)).

When Tamga cannot be asked, the person is told before they try to send. If
the extension has no address or key, a notice says "Tamga is not set up in
this browser yet", with a button to its settings, as soon as the page loads.
If a paste cannot be checked, a notice says why ("Tamga cannot be reached",
or what else went wrong) and what will happen at the send. These notices
hold nothing. Each reason is said once for a page load, not once for a
paste. Under `failMode: open` the notice says that what is sent from the
page goes to the site unchecked.

## What it does not cover

- **Other browsers.** Chrome and Edge from version 111, and Firefox from
  140 through the package `node tools/package.js firefox` makes ([Firefox](#firefox);
  not yet run in Firefox). Not Safari.
- **Other sites.** Only `https://claude.ai`, `https://chatgpt.com` and
  `https://gemini.google.com`. Not any other chat site, not the same
  products under another address (an enterprise tenant on its own domain, a
  model inside another product such as an office suite or a search page).
- **The sites' mobile and desktop apps.** They are not browsers.
- **A person who removes it or turns it off**, unless it is force-installed
  by policy. A person who uses another browser or another device is not
  covered either, nor is a private window, where extensions are off unless
  the person allows them there (private windows can be switched off by
  policy: `IncognitoModeAvailability` in Chrome, `InPrivateModeAvailability`
  in Edge).
- **Files that are not text, and large ones.** A PDF, an image, an office
  document or an archive is not read, nor is a text file over 200 KB under
  `failMode: open` (under `closed` it is not attached). A file renamed to a
  name the extension does not read is not read either. A file input the site
  never puts in the page, a file the site reads some other way than the
  input's events, a drop or a paste, and a file from the site's own
  cloud-drive picker are not seen at all.
- **Voice input**, and anything else that reaches the site without passing
  through the message box as text.
- **The site's answers.** Only what the person sends is checked.
- **Text sent faster than the check on a broken adapter.** If a site's
  markup changed so that neither its adapter nor the generic fallback finds
  the message box, the extension shows "Tamga could not attach to this
  page" and, under `failMode: closed`, holds a click on anything that looks
  like a send button. It cannot hold a key press in an element it cannot
  see as a text box, or a send the page makes from an event the extension
  does not listen for. A content script cannot do better than that.
- **A send button the fallback cannot tell.** When a site's own selectors
  stop matching, the fallback finds a send button that is in a form, or
  named in English, or named "send" in one of eight other languages, or seen
  to appear only when the box holds text
  ([the list and the limits](../extension/README.md#site-adapters)). A
  button that is none of those is not held; a send by Enter still is.
- **A site that reads the box as you type.** The check happens at paste and
  at send. If a site transmits a draft before the person sends it, that
  draft is not stopped.
- **Conversation context.** Each text is judged as one user message. An
  injection or a secret split over several messages is judged piece by
  piece ([where the scan verdict differs from the proxy
  path](operations.md#checking-a-text-without-sending-it)).
- **Text longer than about 250 KB**, the scan API's body limit, and, with
  the [inline classifier](operations.md#inline-classifier) on, clean text
  longer than `scan.classifier.max_chars`: the scan API gives no verdict for
  either, so under `failMode: closed` such a message cannot be sent at all.
- **Enter that is not a send.** Enter without Shift in a message box is
  taken for a send. Where a site uses it for something else, the extension
  checks the text and then presses the send button.

It is a control for people who are not trying to get round it. It is not a
data-loss-prevention boundary.

## What Tamga needs

**A reachable `/api/v1/scan`.** The browsers must be able to reach the Tamga
URL over https (http is accepted for `localhost` only).

**A key of the `proxy` scope**, created as for an application
([keys](operations.md#keys-for-applications)):

```bash
curl -s -X POST "$TAMGA_URL/api/v1/apikeys" \
  -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" -H "Content-Type: application/json" \
  -d '{"label":"browser-extension-finance","scope":"proxy","expires_in_days":90}'
```

Treat this key as known to everyone who has the extension. It sits in the
browser's policy or in the extension's storage, and a person with access to
their own machine can read it. That is why it must be a `proxy` key: in the
management API it opens the scan route and nothing else, and it is told
`custom` or `competitor` in place of the names on your watch list. It does
also open the proxy routes, as any `proxy` key does, counted against that
key. Use a key made for the extension and nothing else, and revoke it when a
group no longer needs it.

**Nothing in `TAMGA_CORS_ORIGINS`.** The scan route refuses a request whose
`Origin` is an `http` or `https` origin that is not on that list, which is
how a web page is kept from spending a key's limit
(`originAllowed` in `proxy/internal/proxy/scan.go`). An `Origin` of any other
scheme is let in. The extension makes its requests from its service worker,
which sends `Origin: chrome-extension://<id>`: observed in Edge 155, together
with `Sec-Fetch-Site: none`, no cookie and no referrer. So the extension's
origin does not have to be listed, and listing it changes nothing. The
`Origin` check is not what protects the route; the key is.

The service worker's request is also not subject to the browser's CORS rules
once the extension holds the host permission for the Tamga origin: in the
same run the browser sent no preflight and read a response that carried no
`Access-Control-Allow-Origin`. Without that permission the request would be
an ordinary cross-origin one and would fail, because Tamga's preflight does
not offer `X-Tamga-Key` to web pages. The extension checks for the
permission before it asks and says "the browser has not allowed this
extension to reach your Tamga server" when it is missing.

No change to the proxy was needed for the extension.

**Enough rate limit.** The scan route allows 60 calls a minute per key, or
`rate_limit.max_requests_per_minute` when the policy sets it (that setting
is the proxy path's limit as well; there is no separate one for scans). Over
the limit the answer is 429, which the extension treats as no verdict: under
`failMode: closed` everyone sharing that key is held until the minute
passes. One message costs one scan, or two when a paste is edited before it
is sent or when a text had to be redacted, and each attached text file one
more. So:

- **Use one key per group, not one for the organisation.** A group of about
  ten people writing at once fits in 60 a minute with room; a department of
  two hundred on one key does not.
- A key per group also lets you revoke one group's key, and the log line
  for each scan names the key.
- A few scans run at a time (about half the processors); beyond that the
  answer is 503, which is also "no verdict".
- One tab may ask for at most 30 scans a minute. That limit is the
  extension's own, so that a page scripting pastes cannot use up the key.

## Settings

| Setting | Values | Default | |
|---|---|---|---|
| `tamgaUrl` | `https://...` | none | Base URL of Tamga. The extension calls `<URL>/api/v1/scan` |
| `tamgaKey` | `tk_...` | none | A `proxy` key |
| `failMode` | `closed`, `open` | `closed` | What happens to a send when there is no verdict |
| `autoRedact` | `true`, `false` | `false` | Replace the text without asking. The person is told either way |
| `sites` | list of `claude`, `chatgpt`, `gemini` | all three | Sites the extension acts on |
| `timeoutMs` | 500 to 10000 | 2500 | How long to wait for Tamga |

A setting made by policy wins over one typed into the options page and is
shown there as locked. With no URL and no key the extension is not
configured, which under `failMode: closed` means nothing can be sent on the
three sites until it is.

The first check after the browser starts was slower than the rest in the
runs made here: about one second against a local stand-in, once more than
1.5 seconds, where later checks took a few milliseconds. The default timeout
leaves room for that. If your Tamga is far from the browsers, raise
`timeoutMs` rather than switching to `failMode: open`.

## Installing it for an organisation

None of this section was run here: no extension was packed, no policy was
set, and managed storage was exercised only through a fake in the tests. It
is written from Chrome's and Edge's enterprise documentation. Try it on one
machine first.

### 1. Add your Tamga origin to the manifest

The extension has to hold a host permission for your Tamga origin. A person
installing it by hand grants that with a click in the options page. A
force-installed extension has nobody to click, and we know of no policy that
grants an optional host permission. Since you pack the extension yourself,
put the origin in the manifest before you pack, in `extension/manifest.json`:

```json
"host_permissions": [
  "https://claude.ai/*",
  "https://chatgpt.com/*",
  "https://gemini.google.com/*",
  "https://tamga.internal/*"
],
```

and, to hold the extension to that one address, narrow `connect-src` in
`content_security_policy` from `https:` to `https://tamga.internal`.
`node --test` will then fail the test that pins `host_permissions` to the
three sites; that test describes the repository's copy, not yours.

If you would rather not edit the manifest, each person has to open the
extension's options once and press Save, which makes the browser ask for
that origin.

### 2. Pack and sign it

```bash
# First time: creates extension.crx and extension.pem. Keep the .pem safe.
chrome --pack-extension=/path/to/tamga/extension

# Later versions: the same key keeps the same extension ID.
chrome --pack-extension=/path/to/tamga/extension --pack-extension-key=/path/to/extension.pem
```

The extension ID is derived from that key: 32 letters, shown on
`chrome://extensions` when the `.crx` or the unpacked directory is loaded.
Raise `version` in `manifest.json` (and in `package.json`; a test keeps the
two equal) for every release you pack. `test/`, `tools/`, `README.md` and
`package.json` are not needed in the package: `node tools/package.js
chromium` in `extension/` writes the extension without them to
`dist/chromium/`, which you can pack in place of the source directory.

### 3. Host it

Serve the `.crx` and an update manifest from a web server the browsers can
reach:

```xml
<?xml version='1.0' encoding='UTF-8'?>
<gupdate xmlns='http://www.google.com/update2/response' protocol='2.0'>
  <app appid='YOUR_EXTENSION_ID'>
    <updatecheck codebase='https://software.example.internal/tamga/tamga-0.9.0.crx' version='0.9.0' />
  </app>
</gupdate>
```

On Windows and macOS, Chrome and Edge install an extension from outside
their stores only on a machine that is managed (joined to a domain or
enrolled in device management).

### 4. Force-install it

Either policy does it. `ExtensionInstallForcelist` takes
`<id>;<update manifest URL>`:

```json
{
  "ExtensionInstallForcelist": [
    "YOUR_EXTENSION_ID;https://software.example.internal/tamga/updates.xml"
  ]
}
```

`ExtensionSettings` does the same and can say more:

```json
{
  "ExtensionSettings": {
    "YOUR_EXTENSION_ID": {
      "installation_mode": "force_installed",
      "update_url": "https://software.example.internal/tamga/updates.xml",
      "toolbar_pin": "force_pinned"
    }
  }
}
```

A force-installed extension cannot be removed or turned off by the person.
The policy names are the same in Edge.

Where these go:

| | Chrome | Edge |
|---|---|---|
| Windows | Group Policy, or `HKLM\Software\Policies\Google\Chrome` | Group Policy, or `HKLM\Software\Policies\Microsoft\Edge` |
| macOS | a configuration profile for `com.google.Chrome` | a configuration profile for `com.microsoft.Edge` |
| Linux | a JSON file in `/etc/opt/chrome/policies/managed/` | a JSON file in `/etc/opt/edge/policies/managed/` |

### 5. Set its settings (managed storage)

The settings are described by `extension/managed_schema.json`. The values:

```json
{
  "tamgaUrl": "https://tamga.internal",
  "tamgaKey": "tk_your_key_here",
  "failMode": "closed",
  "autoRedact": false,
  "sites": ["claude", "chatgpt", "gemini"],
  "timeoutMs": 2500
}
```

**Linux**, as a file in the managed policy directory:

```json
{
  "3rdparty": {
    "extensions": {
      "YOUR_EXTENSION_ID": {
        "tamgaUrl": "https://tamga.internal",
        "tamgaKey": "tk_your_key_here",
        "failMode": "closed",
        "autoRedact": false,
        "sites": ["claude", "chatgpt", "gemini"]
      }
    }
  }
}
```

**Windows**, in the registry (Chrome shown; for Edge the path starts
`HKLM\Software\Policies\Microsoft\Edge`). `sites` is left out here, which
keeps all three:

```
Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\Policies\Google\Chrome\3rdparty\extensions\YOUR_EXTENSION_ID\policy]
"tamgaUrl"="https://tamga.internal"
"tamgaKey"="tk_your_key_here"
"failMode"="closed"
"autoRedact"=dword:00000000
```

**macOS**, as a configuration profile whose payload is for the preference
domain `com.google.Chrome.extensions.YOUR_EXTENSION_ID` (for Edge,
`com.microsoft.Edge.extensions.YOUR_EXTENSION_ID`), with the settings as its
keys:

```xml
<key>tamgaUrl</key>
<string>https://tamga.internal</string>
<key>tamgaKey</key>
<string>tk_your_key_here</string>
<key>failMode</key>
<string>closed</string>
<key>autoRedact</key>
<false/>
```

Of the three, the macOS form is the one we are least sure of; check it
against Chromium's "Configuring Apps and Extensions by Policy".

`chrome://policy` (or `edge://policy`) shows what the browser read, under
the extension's name. Give different groups different keys by applying a
different `tamgaKey` to each group's machines or users.

### Firefox

None of this was run: no Firefox build was signed or installed by policy.
It is written from Mozilla's documentation (the enterprise policy
templates, the WebExtensions manifest reference, and the add-on signing and
data-consent pages). Try it on one machine first.

**1. The Tamga origin and the add-on ID.** Add your Tamga origin to
`host_permissions` in `extension/manifest.json` as in step 1 above; the
Firefox manifest is made from it. Firefox needs an add-on ID for managed
storage and for policy, and an ID is unique across Mozilla's signing
service, so choose one of your own:

```bash
cd extension
node tools/package.js firefox --gecko-id tamga-chat-sites@corp.example
# writes dist/firefox/ and dist/tamga-firefox-<version>.zip
```

**2. Sign it.** Release and ESR builds of Firefox install only extensions
signed by Mozilla. Submit `dist/tamga-firefox-<version>.zip` for
self-distribution ("on your own", unlisted) on addons.mozilla.org, by its
web form or with Mozilla's `web-ext sign`; it is signed without being listed
and you get the signed `.xpi` back. Raise `version` for every release; a
signed version cannot be signed again. The manifest declares that the
extension sends website content off the device (to your Tamga), which
Firefox shows to a person who installs it by hand.

**3. Force-install it and give it its settings**, in `policies.json`:

```json
{
  "policies": {
    "ExtensionSettings": {
      "tamga-chat-sites@corp.example": {
        "installation_mode": "force_installed",
        "install_url": "file:///opt/tamga/tamga-chat-sites-0.9.0.xpi"
      }
    },
    "3rdparty": {
      "Extensions": {
        "tamga-chat-sites@corp.example": {
          "tamgaUrl": "https://tamga.internal",
          "tamgaKey": "tk_your_key_here",
          "failMode": "closed",
          "autoRedact": false,
          "sites": ["claude", "chatgpt", "gemini"]
        }
      }
    }
  }
}
```

`install_url` is where Firefox fetches the signed `.xpi`: a `file:///` path
is documented, and an https address on your own server is the usual
alternative (not tried here). Firefox reinstalls when the version inside
the `.xpi` changes. The `3rdparty` values are what the extension reads as
managed storage, field by field as in Chrome; Firefox does not check them
against `managed_schema.json`. A change to them takes effect after Firefox
restarts.

Where `policies.json` goes: in a `distribution` directory next to the
Firefox executable on Windows, in `Firefox.app/Contents/Resources/distribution`
on macOS, and in `/etc/firefox/policies/` (or `distribution` in the
installation directory) on Linux. On Windows the same policies can be set by
Group Policy with Mozilla's ADMX templates (`ExtensionSettings` takes the
JSON above as one value); on macOS by a configuration profile for
`org.mozilla.firefox`. `about:policies` shows what Firefox read.

To keep private windows covered, either let the extension run in them
(`"private_browsing": true` in its `ExtensionSettings` entry) or switch
them off (`DisablePrivateBrowsing`).

Known differences from Chrome, none tried: a person can withdraw a Manifest
V3 extension's access to a site in `about:addons` (whether a force-installed
one can was not checked; if access is withdrawn the content script does not
run on that site and nothing is checked); Firefox runs the worker's code as
an event page; and whether a site accepts the `drop` and `paste` events the
extension dispatches after checking a file is a live-test item.

### Trying it without any of that

`chrome://extensions`, Developer mode, "Load unpacked", choose the
`extension/` directory, then fill in the options page. That is enough for
one person or an evaluation, and it is how the extension was run here. In
Firefox: `node tools/package.js firefox`, then `about:debugging`, "This
Firefox", "Load Temporary Add-on...", and `dist/firefox/manifest.json`; it is
gone when Firefox closes.

## What is recorded

**At Tamga: nothing about the text, and nothing in the console.** The scan
route stores nothing. A scan publishes no event, so it does not reach the
request store, the audit log, the live feed or the alerts, and the console
shows nothing about browser checks: not how many, not what was blocked, not
for whom. What there is: one log line per call from the proxy (the key's id,
the size, the verdict, the count of findings by type, never the text) and
the scan counter in the metrics.

That is a real limit. If someone is stopped from pasting a customer list
into a chat site, nobody but that person learns of it unless you collect the
proxy's log and read the `scan api call` lines by `key_id`.

**In the browser: nothing.** The extension keeps no history and writes no
text to storage. A text that passed is held in memory for at most 30 seconds
and one that did not for 10, so that a second key press does not cost a
second scan. It has no analytics and makes no request other than the scan.

If you need browser checks in the console, this is what it would take, and
none of it is built:

- A way for a scan to be recorded as a request is: the
  [LiteLLM route](litellm.md) already does this for its calls, storing the
  verdict and the findings without the text. The scan route would need the
  same behind a setting, with a provider name such as `browser` and the
  site as the model.
- Something that says who. One key per group identifies the group. A person
  would need a key each, or an identity the extension can prove, which a
  header the browser fills in is not.
- A decision that is not technical: recording what employees were stopped
  from sending to a chat site is monitoring of employees, with the notice,
  consultation and retention rules that brings where you operate.

## What the extension can see and send

For whoever has to approve it:

- It runs on the three sites and nowhere else, in the top frame only.
- It reads the text of the message box at paste and at send, and the
  content of a text file the person attaches, before the site has it. It
  does not read the conversation, the site's answers, cookies or anything
  else on the page.
- That text goes to the Tamga URL you configured, over https, from the
  extension's service worker, and nowhere else. A redirect is treated as an
  error and not followed.
- The key is read by the service worker and by the options page. It is
  never given to a content script, never put in the page, never written to
  the site's storage and never logged; the options page does not show a
  stored key.
- It loads no code from the network. Its content security policy allows
  scripts from the package only.
- Its permissions are `storage` and the three sites, plus the one Tamga
  origin.
- The sites can tell that it is installed: the files of its content script
  are readable by those three origins, as an extension that imports its own
  modules into a page has to allow.

## When a site changes its page

The badge "the claude adapter did not match this page. Using the generic
fallback" means the site's own selectors stopped matching and the extension
is working from a guess. "Tamga could not attach to this page" means it
found no message box at all. In both cases the fix is in one file under
`extension/src/content/adapters/`; see
[`extension/README.md`](../extension/README.md#when-a-site-changes). Then
raise the version, pack again and let the browsers update.
