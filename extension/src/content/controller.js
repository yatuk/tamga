// What happens on a paste and on a send. The controller is given the page
// (doc), the site's adapter, something to show notices with (ui) and a way
// to ask the service worker (ask); it never touches chrome.* or the network
// itself, and it never keeps a text longer than the short approval below.
//
// Send: the event is stopped before the page sees it, the text in the box is
// checked, and only a text that was checked is let through. Paste: the paste
// lands, the box is checked straight away, and the person is told early.
// Attach (a file input's change, a drop or a paste that carries files): the
// event is stopped before the page sees it, each text file is read and
// checked, and only then is the event given to the page again. A file that
// is not text is not read: the attach goes on and the person is told once.

import { MAX_TEXT_CHARS, MSG_CONFIG, MSG_OPEN_OPTIONS, MSG_SCAN, parseConfigReply, parseScanReply } from '../shared/protocol.js';
import { first } from './adapters/base.js';
import { attachState, generic, observePage, resolveBox, resolveFileInput, resolveSend } from './adapters/index.js';
import { decide, isFileReason, needsSetup, reasonText } from './decide.js';
import { FILE_MAX_BYTES, FILE_MAX_COUNT, filesOf, isTextFile, readTextFile, safeName, scanTextFor, signature } from './files.js';

// A text that was checked may be sent for this long without a second check,
// so that a paste followed by Enter is one scan and not two.
export const APPROVAL_MS = 30000;
// One send is several events (a key press, the click the page makes of it, a
// form's submit). Once the first is let through the rest follow within this.
export const GESTURE_MS = 1500;
// A verdict that stops a text is shown again without a new scan for this long.
export const REMEMBER_MS = 10000;
export const SLOW_CHECK_MS = 500;
export const RESEND_CHECK_MS = 800;
export const ATTACH_POLL_MS = 2000;
export const ATTACH_GRACE_POLLS = 4;
// The events loader.js listens for on window, in the capture phase. It cannot
// import this list (a content script is not a module); a test keeps the two
// the same.
export const EVENT_TYPES = Object.freeze(['keydown', 'click', 'submit', 'paste', 'input', 'change', 'drop']);

const CLOSED = Object.freeze({ failMode: 'closed', autoRedact: false });

// Reasons that say Tamga itself did not answer, as opposed to something
// about this one text or this one minute.
const UNREACHED = Object.freeze(['timeout', 'network', 'unavailable']);

function targetOf(event) {
  const path = typeof event.composedPath === 'function' ? event.composedPath() : null;
  return (path && path[0]) || event.target;
}

function isSendKey(event) {
  return event.key === 'Enter' && !event.shiftKey && !event.altKey && !event.isComposing && event.keyCode !== 229;
}

function hold(event) {
  event.preventDefault();
  event.stopImmediatePropagation();
}

export function createController({ doc, site, ui, ask, timers = globalThis, now = Date.now }) {
  let enabled = true;
  let behaviour = CLOSED; // closed until the worker says otherwise
  let approval = null; // { text, until }
  let remembered = null; // { text, outcome, until }
  let checking = null; // the text being checked
  let seq = 0;
  let missing = 0;
  let told = null; // the reason for having no verdict the person was last told
  let fileApproval = null; // { sigs, until }: files that were checked and given to the page
  let fileHold = null; // { source, key }: the attach being checked, or held after its check
  let fileSeq = 0;
  let binaryTold = false;
  // Events the extension dispatched itself, to give a checked attach to the
  // page: they are not checked again.
  const own = new WeakSet();

  const failOpen = () => behaviour.failMode === 'open';

  function approve(text) {
    approval = { text, until: now() + APPROVAL_MS };
    remembered = null;
  }

  const isApproved = (text) => Boolean(approval) && approval.text === text && now() < approval.until;

  /** Lets one send of an approved text through, and the events that belong to it. */
  function approved(text) {
    if (!isApproved(text)) return false;
    approval.until = Math.min(approval.until, now() + GESTURE_MS);
    return true;
  }

  function show(notice, onAction) {
    ui.show(notice, (id) => {
      ui.hide();
      if (onAction) onAction(id);
    });
  }

  function openSettings() {
    ask({ type: MSG_OPEN_OPTIONS }).catch(() => {});
  }

  /**
   * Says, without holding anything, that Tamga cannot be asked on this page
   * and what follows from that. Once for a reason: a second paste that fails
   * the same way says nothing, and a different reason is said again.
   */
  function tellUnchecked(reason, open) {
    if (told === reason) return;
    told = reason;
    const setup = needsSetup(reason);
    const title = setup
      ? 'Tamga is not set up in this browser yet'
      : UNREACHED.includes(reason)
        ? 'Tamga cannot be reached'
        : 'Tamga could not check this text';
    const why = reason === 'not_configured' ? 'It has no Tamga address or key.' : reasonText(reason);
    const then = open
      ? 'Your administrator allows sending without a check, so what you send from this page goes to the site unchecked.'
      : setup
        ? 'Nothing can be sent from this page until it is set up.'
        : 'A message is held when you send it, until it can be checked.';
    show(
      {
        tone: 'note',
        quiet: true,
        title,
        body: `${why} ${then}`,
        actions: setup
          ? [
              { id: 'setup', label: 'Open settings' },
              { id: 'ok', label: 'Close' },
            ]
          : [{ id: 'ok', label: 'OK' }],
      },
      (id) => {
        if (id === 'setup') openSettings();
      },
    );
  }

  function resend(ctx, text) {
    const button =
      ctx.button || ctx.adapter.findSendButton(doc, ctx.composer) || generic.findSendButton(doc, ctx.composer);
    if (button && !button.disabled && typeof button.click === 'function') {
      button.click();
    } else {
      const win = doc.defaultView;
      ctx.composer.dispatchEvent(
        new win.KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true }),
      );
    }
    // A page may ignore a click or a key press that a script made. If the
    // text is still in the box, say so; the next send needs no second check.
    timers.setTimeout(() => {
      if (ctx.composer.isConnected === false || ctx.adapter.readText(ctx.composer) !== text) return;
      approve(text);
      show({
        tone: 'note',
        title: 'Checked by Tamga: nothing found',
        body: 'The page did not take the send. Press Enter or the send button again.',
        actions: [{ id: 'ok', label: 'OK' }],
      });
    }, RESEND_CHECK_MS);
  }

  function replace(ctx, outcome) {
    if (ctx.adapter.replaceText(ctx.composer, outcome.redactedText)) {
      remembered = null;
      show({
        tone: 'change',
        title: 'Tamga changed your text',
        body: `Your organisation's policy does not let these through: ${outcome.found}. They were replaced with placeholders. Read the text, then send it.`,
        actions: [{ id: 'ok', label: 'OK' }],
      });
      return;
    }
    show({
      tone: 'stop',
      title: 'Tamga could not change the text in the box',
      body: `Your organisation's policy does not let these through: ${outcome.found}. The text as it stands is not sent. This is the version with placeholders; copy it in by hand.`,
      detail: outcome.redactedText,
      actions: [{ id: 'ok', label: 'OK' }],
    });
  }

  function blockBody(outcome) {
    switch (outcome.why) {
      case 'strip':
        // STRIP takes the whole piece of text out, not the part that was
        // found. For a message a person typed that leaves nothing to send.
        return (
          `Your organisation's policy removes a text entirely when it finds this in it: ${outcome.found || 'what a rule names'}. ` +
          'There is no changed version to send in its place, so nothing is sent. Take that part out and send again.'
        );
      case 'no_text':
        return `Your organisation's policy changes text like this before it is sent (${outcome.found || 'policy'}), and Tamga returned no changed text. Nothing is sent. Take that part out and send again.`;
      default:
        return `Your organisation's policy does not allow it: ${outcome.found || 'policy'}. Remove that part to send.`;
    }
  }

  function apply(outcome, text, ctx, intent) {
    const sending = intent === 'send';
    switch (outcome.kind) {
      case 'pass':
        approve(text);
        ui.hide(); // also a notice about an earlier text
        if (sending) resend(ctx, text);
        return;

      case 'warn':
        remembered = { text, outcome, until: now() + REMEMBER_MS };
        show(
          {
            tone: 'note',
            title: 'Tamga: policy warning',
            body: `This text contains: ${outcome.found || 'something your organisation asks you to check'}. You may still send it.`,
            actions: sending
              ? [
                  { id: 'continue', label: 'Send anyway' },
                  { id: 'cancel', label: 'Keep editing' },
                ]
              : [{ id: 'continue', label: 'Understood' }],
          },
          (id) => {
            if (id !== 'continue' || ctx.adapter.readText(ctx.composer) !== text) return;
            approve(text);
            if (sending) resend(ctx, text);
          },
        );
        return;

      case 'block':
        remembered = { text, outcome, until: now() + REMEMBER_MS };
        show({
          tone: 'stop',
          title:
            outcome.why === 'strip'
              ? 'Tamga: the policy removes this whole text'
              : sending
                ? 'Tamga stopped this message'
                : 'Tamga will not let this text be sent',
          body: blockBody(outcome),
          actions: [{ id: 'ok', label: 'OK' }],
        });
        return;

      case 'redact':
        if (outcome.auto) {
          replace(ctx, outcome);
          return;
        }
        remembered = { text, outcome, until: now() + REMEMBER_MS };
        show(
          {
            tone: 'change',
            title: 'Tamga: this text has to change before it is sent',
            body: `Your organisation's policy does not let these through: ${outcome.found}. This is the text with placeholders in their place.`,
            detail: outcome.redactedText,
            actions: [
              { id: 'replace', label: 'Use this text' },
              { id: 'cancel', label: 'Keep editing' },
            ],
          },
          (id) => {
            if (id === 'replace' && ctx.adapter.readText(ctx.composer) === text) replace(ctx, outcome);
          },
        );
        return;

      default: // unavailable
        if (!sending) {
          // A paste is not held, and the send will be held or let through by
          // the fail mode. The person is told now, once, not at the send.
          tellUnchecked(outcome.reason, outcome.failOpen);
          return;
        }
        told = outcome.reason; // both notices below say it
        if (outcome.failOpen) {
          approve(text);
          resend(ctx, text);
          show({
            tone: 'note',
            title: 'Tamga did not check this message',
            body: `${reasonText(outcome.reason)} Your administrator allows sending without a check, so it was sent.`,
            actions: [{ id: 'ok', label: 'OK' }],
          });
          return;
        }
        show(
          {
            tone: 'stop',
            title: 'Tamga is holding this message',
            body:
              `${reasonText(outcome.reason)} Nothing is sent until the text has been checked.` +
              (outcome.retryAfterS ? ` Try again in ${outcome.retryAfterS} s.` : ''),
            actions: [
              { id: 'retry', label: 'Try again' },
              ...(needsSetup(outcome.reason) ? [{ id: 'setup', label: 'Open settings' }] : []),
              { id: 'cancel', label: 'Close' },
            ],
          },
          (id) => {
            if (id === 'setup') {
              openSettings();
            } else if (id === 'retry') {
              const current = ctx.adapter.readText(ctx.composer);
              if (current.trim() !== '') check(current, ctx, 'send');
            }
          },
        );
    }
  }

  async function check(text, ctx, intent) {
    const mine = ++seq;
    checking = text;
    const slow =
      intent === 'send'
        ? timers.setTimeout(() => {
            ui.show({ tone: 'note', title: 'Tamga is checking this message', body: '', actions: [] }, () => {});
          }, SLOW_CHECK_MS)
        : null;
    let raw;
    try {
      raw =
        text.length > MAX_TEXT_CHARS
          ? { ok: false, behaviour, reason: 'too_large' }
          : await ask({ type: MSG_SCAN, text, intent });
    } catch {
      raw = undefined; // the worker did not answer: no verdict
    }
    if (slow !== null) timers.clearTimeout(slow);
    if (mine !== seq) return; // a newer check has taken over
    checking = null;

    const outcome = decide(raw, behaviour);
    const reply = parseScanReply(raw);
    if (reply) behaviour = reply.behaviour;

    if (ctx.adapter.readText(ctx.composer) !== text) {
      // The verdict is for a text that is no longer in the box.
      ui.hide();
      if (intent === 'send') {
        show({
          tone: 'note',
          title: 'Tamga: the text changed during the check',
          body: 'Nothing was sent. Send again to have the text as it is now checked.',
          actions: [{ id: 'ok', label: 'OK' }],
        });
      }
      return;
    }
    apply(outcome, text, ctx, intent);
  }

  // ---------- attachments ----------
  //
  // An attach is { source, files, text, other, key, checking }: source is
  // { kind: 'input', input } for a file input, { kind: 'drop' | 'paste',
  // target, strings } for a drop or a paste, with hasText for a paste that
  // also carries text. text and other are the files that are read and the
  // ones that are not.

  const plural = (n, one, many) => (n === 1 ? one : many);

  function approveFiles(files) {
    fileApproval = { sigs: new Set(files.map(signature)), until: now() + APPROVAL_MS };
  }

  const filesApproved = (files) =>
    Boolean(fileApproval) && now() < fileApproval.until && files.every((f) => fileApproval.sigs.has(signature(f)));

  /** Says, once for a page load, that a file that is not text was not read. */
  function tellBinary(files) {
    if (binaryTold || files.length === 0) return;
    binaryTold = true;
    show({
      tone: 'note',
      quiet: true,
      title: 'Tamga does not read this kind of file',
      body:
        `${files.map((f) => safeName(f.name)).join(', ')} ${plural(files.length, 'was', 'were')} not checked. ` +
        `Tamga reads text files (plain text, Markdown, CSV, JSON, YAML, XML, logs, source code) of up to ${FILE_MAX_BYTES / 1024} KB. ` +
        'PDF files, images and office documents are not read, and go to the site as they are.',
      actions: [{ id: 'ok', label: 'OK' }],
    });
  }

  /**
   * Gives a checked attach to the page: the same files, in an event of the
   * same kind, which the extension lets through. Returns false when the
   * browser would not build that event.
   */
  function release(attach) {
    const { source, files } = attach;
    approveFiles(files);
    if (fileHold === attach) fileHold = null;
    const win = doc.defaultView;
    try {
      if (source.kind === 'input') {
        // The input still holds the files: the page reads them from it.
        for (const type of ['input', 'change']) {
          const event = new win.Event(type, { bubbles: true });
          own.add(event);
          source.input.dispatchEvent(event);
        }
        return true;
      }
      // A drop's or a paste's own data cannot be read once its event is
      // over, so the page is handed a new one holding the same files.
      const data = new win.DataTransfer();
      for (const file of files) data.items.add(file);
      for (const [type, value] of source.strings) data.setData(type, value);
      const event =
        source.kind === 'drop'
          ? new win.DragEvent('drop', { bubbles: true, cancelable: true, composed: true, dataTransfer: data })
          : new win.ClipboardEvent('paste', { bubbles: true, cancelable: true, composed: true, clipboardData: data });
      if (!(event.dataTransfer || event.clipboardData)) throw new Error('the event carries no data');
      own.add(event);
      const target = source.target && source.target.isConnected !== false ? source.target : doc.body;
      target.dispatchEvent(event);
      return true;
    } catch {
      // The approval above lets the same files through when they are added again.
      show({
        tone: 'note',
        title: 'Checked by Tamga: nothing found',
        body: 'The page could not be handed the file. Add it again; it will not be checked a second time.',
        actions: [{ id: 'ok', label: 'OK' }],
      });
      return false;
    }
  }

  /** The attach does not reach the page. A file input is emptied. */
  function discard(attach) {
    if (fileHold === attach) fileHold = null;
    if (attach.source.kind !== 'input') return;
    try {
      attach.source.input.value = '';
    } catch {
      // The page never saw the files either way.
    }
  }

  const named = (r) => `${r.name} (${r.outcome.found || 'policy'})`;

  function applyFiles(attach, results) {
    const n = attach.files.length;
    const notAttached = n === 1 ? 'It is not attached.' : `None of the ${n} files is attached.`;

    // A file the policy stops, or would change: nothing of the attach goes.
    // The extension does not write a changed copy of a person's file.
    const stopped = results.find((r) => r.outcome.kind === 'block' || r.outcome.kind === 'redact');
    if (stopped) {
      discard(attach);
      const { kind, why } = stopped.outcome;
      let title;
      let body;
      if (kind === 'redact' || why === 'no_text') {
        title = 'Tamga: this file has to change before it is sent';
        body =
          `Your organisation's policy changes text like this before it is sent: ${named(stopped)}. Tamga does not change files. ${notAttached} ` +
          'Take those parts out of the file and attach it again, or paste its text into the message box, where Tamga can offer the changed text.';
      } else if (why === 'strip') {
        title = 'Tamga: the policy removes this whole file';
        body = `Your organisation's policy removes a text entirely when it finds this in it: ${named(stopped)}. ${notAttached}`;
      } else {
        title = 'Tamga stopped this file';
        body = `Your organisation's policy does not allow it: ${named(stopped)}. ${notAttached} Remove that part from the file to attach it.`;
      }
      show({ tone: 'stop', title, body, actions: [{ id: 'ok', label: 'OK' }] });
      return;
    }

    const unchecked = results.find((r) => r.outcome.kind === 'unavailable');
    if (unchecked) {
      const { reason, failOpen: open, retryAfterS } = unchecked.outcome;
      const local = isFileReason(reason);
      if (!local) told = reason; // the notice below says it
      const why = local ? `${unchecked.name}: ${reasonText(reason)}` : reasonText(reason);
      if (open) {
        release(attach);
        show({
          tone: 'note',
          title: `Tamga did not check this ${plural(n, 'file', 'attachment')}`,
          body: `${why} Your administrator allows sending without a check, so it was attached.`,
          actions: [{ id: 'ok', label: 'OK' }],
        });
        return;
      }
      if (local) {
        discard(attach);
        show({ tone: 'stop', title: 'Tamga could not check this file', body: `${why} ${notAttached}`, actions: [{ id: 'ok', label: 'OK' }] });
        return;
      }
      show(
        {
          tone: 'stop',
          title: `Tamga is holding this ${plural(n, 'file', 'attachment')}`,
          body:
            `${why} ${n === 1 ? 'It is' : 'The files are'} not attached until checked.` +
            (retryAfterS ? ` Try again in ${retryAfterS} s.` : ''),
          actions: [
            { id: 'retry', label: 'Try again' },
            ...(needsSetup(reason) ? [{ id: 'setup', label: 'Open settings' }] : []),
            { id: 'cancel', label: 'Do not attach' },
          ],
        },
        (id) => {
          if (id === 'retry' && fileHold === attach) {
            checkFiles(attach);
            return;
          }
          if (id === 'setup') openSettings();
          discard(attach);
        },
      );
      return;
    }

    const warned = results.filter((r) => r.outcome.kind === 'warn');
    if (warned.length > 0) {
      show(
        {
          tone: 'note',
          title: 'Tamga: policy warning',
          body: `${plural(warned.length, 'This file contains', 'These files contain')} something your organisation asks you to check: ${warned.map(named).join('; ')}. You may still attach ${plural(n, 'it', 'them')}.`,
          actions: [
            { id: 'attach', label: 'Attach anyway' },
            { id: 'cancel', label: 'Do not attach' },
          ],
        },
        (id) => {
          if (id === 'attach' && fileHold === attach) {
            if (release(attach) && !attach.source.hasText) tellBinary(attach.other);
            return;
          }
          discard(attach);
        },
      );
      return;
    }

    ui.hide(); // the "checking" notice, or a notice about an earlier attach
    if (release(attach) && !attach.source.hasText) tellBinary(attach.other);
  }

  /** Reads and checks each text file of an attach, one at a time, and acts on the lot. */
  async function checkFiles(attach) {
    const mine = ++fileSeq;
    attach.checking = true;
    fileHold = attach;
    const slow = timers.setTimeout(() => {
      ui.show({ tone: 'note', title: `Tamga is checking the ${plural(attach.text.length, 'file', 'files')}`, body: '', actions: [] }, () => {});
    }, SLOW_CHECK_MS);
    const results = [];
    if (attach.text.length > FILE_MAX_COUNT) {
      results.push({ name: `${attach.text.length} files`, outcome: decide({ ok: false, behaviour, reason: 'too_many_files' }, behaviour) });
    }
    for (const file of attach.text.length > FILE_MAX_COUNT ? [] : attach.text) {
      const name = safeName(file.name);
      const read = await readTextFile(file);
      let raw;
      if (!read.ok) {
        raw = { ok: false, behaviour, reason: read.reason };
      } else if (read.text.trim() === '') {
        raw = { ok: true, behaviour, verdict: { action: 'PASS', findings: [] } }; // nothing in it to check
      } else {
        try {
          raw = await ask({ type: MSG_SCAN, text: scanTextFor(name, read.text), intent: 'file' });
        } catch {
          raw = undefined; // the worker did not answer: no verdict
        }
      }
      const reply = parseScanReply(raw);
      if (reply) behaviour = reply.behaviour;
      const outcome = decide(raw, behaviour);
      results.push({ name, outcome });
      // One file stopped stops the attach: the rest need not cost a scan.
      if (mine !== fileSeq || outcome.kind === 'block' || outcome.kind === 'redact') break;
    }
    timers.clearTimeout(slow);
    if (mine !== fileSeq) return; // a newer attach has taken over
    attach.checking = false;
    applyFiles(attach, results);
  }

  /**
   * An attach is about to reach the page. Returns true when its event was
   * held for a check, false when it goes on: no text file in it, or the same
   * files were checked a moment ago.
   */
  function onFiles(event, source, files) {
    const text = files.filter(isTextFile);
    const other = files.filter((f) => !isTextFile(f));
    if (text.length === 0 || filesApproved(files)) {
      if (!source.hasText) tellBinary(other);
      return false;
    }
    hold(event);
    const key = files.map(signature).join('\u0001');
    // The change event after the input event of the same pick: the check
    // has started already.
    if (fileHold && fileHold.checking && fileHold.key === key && fileHold.source.kind === source.kind) return true;
    checkFiles({ source, files, text, other, key, checking: false });
    return true;
  }

  function onSend(event, ctx) {
    if (!ctx.composer) {
      // A send control, and no box to read: the text cannot be checked.
      if (failOpen()) {
        show({
          tone: 'note',
          title: 'Tamga could not attach to this page',
          body: 'It found no message box, so this message was not checked. Your administrator allows sending without a check.',
          actions: [{ id: 'ok', label: 'OK' }],
        });
        return;
      }
      hold(event);
      show({
        tone: 'stop',
        title: 'Tamga could not attach to this page',
        body: 'It found no message box, so it cannot check what you send. Sending is held. Tell your administrator: the site has probably changed.',
        actions: [{ id: 'ok', label: 'OK' }],
      });
      return;
    }

    const text = ctx.adapter.readText(ctx.composer);
    if (text.trim() === '') return;
    if (approved(text)) return;
    hold(event);
    if (checking === text) return;
    if (remembered && remembered.text === text && now() < remembered.until) {
      apply(remembered.outcome, text, ctx, 'send');
      return;
    }
    check(text, ctx, 'send');
  }

  function onSubmit(event) {
    const form = targetOf(event);
    if (!form || typeof form.contains !== 'function') return;
    const own = site.findComposer(doc);
    if (own && form.contains(own)) {
      onSend(event, { adapter: site, composer: own, button: null });
      return;
    }
    const box = first(form, ['textarea', '[contenteditable="true"]']);
    if (box) onSend(event, { adapter: generic, composer: box, button: null });
  }

  function clipboardStrings(data) {
    const out = [];
    for (const type of ['text/plain', 'text/html']) {
      let value = '';
      try {
        value = data.getData(type);
      } catch {
        value = '';
      }
      if (value) out.push([type, value]);
    }
    return out;
  }

  /** Called for every keydown, click, submit, paste, input, change and drop, in the capture phase. */
  function handle(event) {
    if (!enabled || own.has(event)) return;
    const target = targetOf(event);
    switch (event.type) {
      case 'keydown': {
        if (!isSendKey(event)) return;
        const box = resolveBox(site, doc, target);
        if (box) onSend(event, { ...box, button: null });
        return;
      }
      case 'click': {
        const send = resolveSend(site, doc, target);
        if (send) onSend(event, send);
        return;
      }
      case 'submit':
        onSubmit(event);
        return;
      case 'input':
      case 'change': {
        // A file input's input and change events are when the page reads
        // the files picked in it. Typing in a text box is an input event
        // too, and is not looked at here.
        const found = resolveFileInput(site, target);
        if (!found) return;
        const files = filesOf(found.input.files);
        if (files.length > 0) onFiles(event, { kind: 'input', input: found.input, hasText: false }, files);
        return;
      }
      case 'drop': {
        // The files can be read from the event only while it runs.
        const files = filesOf(event.dataTransfer?.files);
        if (files.length > 0) onFiles(event, { kind: 'drop', target, strings: [], hasText: false }, files);
        return;
      }
      case 'paste': {
        const data = event.clipboardData;
        const files = filesOf(data?.files);
        if (files.length > 0) {
          const strings = clipboardStrings(data);
          if (onFiles(event, { kind: 'paste', target, strings, hasText: strings.length > 0 }, files)) return;
        }
        const box = resolveBox(site, doc, target);
        if (!box) return;
        // After the paste has landed, so that the whole box is checked.
        timers.setTimeout(() => {
          const text = box.adapter.readText(box.composer);
          if (text.trim() !== '' && checking !== text && !isApproved(text)) check(text, { ...box, button: null }, 'paste');
        }, 0);
        return;
      }
      default:
    }
  }

  function pollAttach() {
    if (!enabled) return;
    observePage(site, doc);
    const state = attachState(site, doc);
    missing = state === 'none' ? missing + 1 : 0;
    if (state === 'none' && missing >= ATTACH_GRACE_POLLS) {
      ui.badge(
        failOpen()
          ? 'Tamga could not attach to this page. Nothing typed here is checked.'
          : 'Tamga could not attach to this page. Sending from here is held.',
        'stop',
      );
    } else if (state === 'generic') {
      ui.badge(`Tamga: the ${site.id} adapter did not match this page. Using the generic fallback.`, 'note');
    } else {
      ui.badge(null);
    }
  }

  /** Asks the worker how to behave and starts watching for a message box. */
  async function start() {
    try {
      const config = parseConfigReply(await ask({ type: MSG_CONFIG }));
      if (config) {
        behaviour = config.behaviour;
        enabled = config.enabled;
        // Known before anything is typed: say it now, not at the first send.
        if (enabled && !config.configured) tellUnchecked('not_configured', failOpen());
      }
    } catch {
      // No answer: stay enabled and closed.
    }
    if (enabled) timers.setInterval(pollAttach, ATTACH_POLL_MS);
    return enabled;
  }

  return { handle, start, pollAttach };
}
