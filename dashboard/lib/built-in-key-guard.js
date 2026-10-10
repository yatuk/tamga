/**
 * Stops a production build that would put an admin key in the browser bundle.
 *
 * A NEXT_PUBLIC_ variable is written into the JavaScript every visitor
 * downloads. NEXT_PUBLIC_ADMIN_KEY is a convenience for `next dev`; a console
 * built with it would hand the admin key to anyone who can load the page.
 * next.config.js calls this with the phase Next.js is in, after the .env
 * files have been read.
 */
const PRODUCTION_BUILD = "phase-production-build";

const MESSAGE = [
  "NEXT_PUBLIC_ADMIN_KEY is set, and this is a production build.",
  "A NEXT_PUBLIC_ value is written into the JavaScript every visitor downloads, so the admin key would be public.",
  "Remove it from the environment and from .env, .env.local and .env.production, then build again.",
  "A production console starts with no key: each person enters one under Settings, and it stays in their browser.",
].join("\n");

/**
 * @param {string} phase The phase next.config.js was called with.
 * @param {Record<string, string | undefined>} [env] The environment to read; the process's own by default.
 */
function refuseBuiltInKey(phase, env = process.env) {
  if (phase === PRODUCTION_BUILD && env.NEXT_PUBLIC_ADMIN_KEY) throw new Error(MESSAGE);
}

module.exports = { refuseBuiltInKey, MESSAGE };
