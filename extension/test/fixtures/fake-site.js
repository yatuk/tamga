// What the fixture pages do in place of a real chat site: send the text in
// the box on Enter, on a click of the send button, or on the form's submit,
// then empty the box. The tests import it; a fixture opened in a browser
// loads it too. Its listeners are on document in the capture phase, ahead of
// anything on the elements, so a send that reaches them was not stopped.

function textOf(el) {
  return el.tagName === 'TEXTAREA' ? el.value : el.textContent;
}

function empty(el) {
  if (el.tagName === 'TEXTAREA') el.value = '';
  else el.textContent = '';
}

/**
 * `deaf: true` makes the page ignore the send button, as a site might ignore
 * a click that a script made.
 */
export function attachFakeSite(doc, { composer, send, deaf = false }) {
  const sent = [];
  const box = () => doc.querySelector(composer);

  function deliver() {
    const el = box();
    if (!el || textOf(el).trim() === '') return;
    sent.push(textOf(el));
    empty(el);
    const log = doc.querySelector('[data-fixture="log"]');
    if (log) log.textContent = `sent ${sent.length}: ${sent[sent.length - 1]}`;
  }

  doc.addEventListener(
    'keydown',
    (event) => {
      const el = box();
      if (event.key !== 'Enter' || event.shiftKey || !el || !el.contains(event.target)) return;
      event.preventDefault();
      deliver();
    },
    true,
  );
  doc.addEventListener(
    'click',
    (event) => {
      const button = event.target.closest ? event.target.closest(send) : null;
      if (!button || deaf) return;
      if (button.getAttribute('type') === 'submit' && button.closest('form')) return; // the submit does it
      deliver();
    },
    true,
  );
  doc.addEventListener(
    'submit',
    (event) => {
      event.preventDefault();
      if (!deaf) deliver();
    },
    true,
  );
  return { sent };
}
