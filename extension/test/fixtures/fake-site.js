// What the fixture pages do in place of a real chat site: send the text in
// the box on Enter, on a click of the send button, or on the form's submit,
// then empty the box; and take a file picked in a file input, dropped on the
// page or pasted, as an upload. The tests import it; a fixture opened in a browser
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
  const attached = []; // the names of the files of each upload, one list per upload
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
  // A file input's change, as a site's upload handler reads it. The input
  // event that comes before it is only counted.
  const inputEvents = [];
  doc.addEventListener(
    'input',
    (event) => {
      if (event.target.tagName === 'INPUT' && event.target.type === 'file') inputEvents.push(event.target);
    },
    true,
  );
  doc.addEventListener(
    'change',
    (event) => {
      const input = event.target;
      if (input.tagName !== 'INPUT' || input.type !== 'file' || input.files.length === 0) return;
      attached.push([...input.files].map((f) => f.name));
    },
    true,
  );
  for (const type of ['drop', 'paste']) {
    doc.addEventListener(
      type,
      (event) => {
        const data = type === 'drop' ? event.dataTransfer : event.clipboardData;
        if (!data || data.files.length === 0) return;
        event.preventDefault();
        attached.push([...data.files].map((f) => f.name));
      },
      true,
    );
  }
  return { sent, attached, inputEvents };
}
