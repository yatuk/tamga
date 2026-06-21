/**
 * OpenAPI-derived types (`npm run gen:api-types` from `../proxy/docs/openapi.yaml`).
 * Hand-written request helpers live in `lib/api/`; migrate field types incrementally from `components`.
 */
export type { components, paths } from "./api-types.generated";
export * from "./api/types-core";
export * from "./api/types-extended";
export * from "./api/types-billing";
export { api } from "./api/client";
