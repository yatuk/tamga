// What happens on a paste and on a send. The controller is given the page
// (doc), the site's adapter, something to show notices with (ui) and a way
// to ask the service worker (ask); it never touches chrome.* or the network
// itself, and it never keeps a text longer than the short approval below.
//
// Send: the event is stopped before the page sees it, the text in the box is
// checked, and only a text that was checked is let through. Paste: the paste
// lands, the box is checked straight away, and the person is told early.

import { MAX_TEXT_CHARS, MSG_CONFIG, MSG_OPEN_OPTIONS, MSG_SCAN, parseConfigReply, parseScanReply } from '../shared/protocol.js';
import { first } from './adapters/base.js';
import { attachState, generic, observePage, resolveBox, resolveSend } from './adapters/index.js';
import { decide, needsSetup, reasonText } from './decide.js';

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

  /** Called for every keydown, click, submit and paste, in the capture phase. */
  function handle(event) {
    if (!enabled) return;
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
      case 'paste': {
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
