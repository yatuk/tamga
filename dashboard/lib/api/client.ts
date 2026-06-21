import { health } from "./endpoints/health";
import { events } from "./endpoints/events";
import { policies } from "./endpoints/policies";
import { webhooks } from "./endpoints/webhooks";
import { apikeys } from "./endpoints/apikeys";
import { patterns } from "./endpoints/patterns";
import { team } from "./endpoints/team";
import { hunts } from "./endpoints/hunts";
import { billing } from "./endpoints/billing";
import { settings } from "./endpoints/settings";
import { exportEndpoints } from "./endpoints/export";

export const api = {
  ...health,
  ...events,
  ...policies,
  ...webhooks,
  ...apikeys,
  ...patterns,
  ...team,
  ...hunts,
  ...billing,
  ...settings,
  ...exportEndpoints,
};

// Re-export types for backward-compatible direct imports from @/lib/api/client
export type { SSOSettings } from "./types-extended";
export type {
  ModelPricing,
  CostBreakdownRow,
  DailyCostRow,
  CostsBreakdownResponse,
} from "./types-billing";
