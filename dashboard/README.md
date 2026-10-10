# Tamga dashboard (Next.js)

## The admin key

The console holds no key of its own. Each person enters one under
**Settings → Access**; it is kept in that browser's localStorage and sent to
the proxy as the `X-Tamga-Admin-Key` header with every call, the live event
feed included. It is never put in a URL. The key can be the proxy's
`TAMGA_ADMIN_KEY` or an API key of scope read, write or admin; what the
console can show and do follows that key's role, and the proxy's refusals
("This key's role cannot read API keys") are shown where they happen.

A console opened with no key says so on every data page and points at
Settings. The "Proxy up" indicator in the header works without one.

The browser talks to the proxy directly, so the proxy must allow the
console's origin: set `TAMGA_CORS_ORIGINS` on the proxy to the exact origin
the console is served from. There is no server-side route in between and no
server-side key: do not give the dashboard container `TAMGA_ADMIN_KEY`.

### `NEXT_PUBLIC_ADMIN_KEY` is for `next dev` only

For local development you may put `NEXT_PUBLIC_ADMIN_KEY=<key>` in
`.env.local`, so that the console works without entering a key. Next.js
writes a `NEXT_PUBLIC_` value into the JavaScript every visitor downloads, so:

- `next build` stops with an error when the variable is set, in the
  environment or in any `.env` file it reads (`lib/built-in-key-guard.js`,
  called from `next.config.js`). Remove it and build again.
- The code names the variable in one place, `lib/built-in-key.ts`, behind
  `process.env.NODE_ENV !== "production"`, so a production bundle has no
  reference to it.
- `.dockerignore` keeps `.env*` out of the image build.
- In development, Settings says the key in use is built into the page, and
  the browser console prints one warning (never the value).

The live Playwright specs (`e2e/live`) read the key for their own API calls
from the `NEXT_PUBLIC_ADMIN_KEY` environment variable or from `.env.local`,
and run against `next dev`, where the built-in key still applies.

## API types (OpenAPI)

Types are generated from the proxy OpenAPI spec:

```bash
cd tamga/dashboard
npm run gen:api-types
```

Source: [`../proxy/docs/openapi.yaml`](../proxy/docs/openapi.yaml). Output: [`lib/api-types.generated.ts`](lib/api-types.generated.ts) (do not edit by hand).

After changing the spec, regenerate and commit both the YAML and the generated file. `npm run check:api-types` fails if the committed generated file is out of date.

Hand-written API helpers live in [`lib/api.ts`](lib/api.ts); they re-export `components` and `paths` from the generated module for gradual typing.
