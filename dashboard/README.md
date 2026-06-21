# Tamga dashboard (Next.js)

## API types (OpenAPI)

Types are generated from the proxy OpenAPI spec:

```bash
cd tamga/dashboard
npm run gen:api-types
```

Source: [`../proxy/docs/openapi.yaml`](../proxy/docs/openapi.yaml). Output: [`lib/api-types.generated.ts`](lib/api-types.generated.ts) (do not edit by hand).

After changing the spec, regenerate and commit both the YAML and the generated file. `npm run check:api-types` fails if the committed generated file is out of date.

Hand-written API helpers live in [`lib/api.ts`](lib/api.ts); they re-export `components` and `paths` from the generated module for gradual typing.
