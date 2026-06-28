import type { WebhookKind } from "@/lib/api";
import type { IntegrationGuide } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";
import { DATADOG, JIRA } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-defs-c";
import { GENERIC, OPSGENIE, PAGERDUTY, SERVICENOW } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-defs-d";
import { SENTINEL, QRADAR } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-defs-b";
import { SLACK, SPLUNK, SPLUNK_HEC, TEAMS } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-defs-a";

export type { IntegrationGuide } from "@/app/(dashboard)/dashboard/integrations/_data/integration-guide-model";

export const GUIDES: Record<WebhookKind, IntegrationGuide> = {
  slack: SLACK,
  teams: TEAMS,
  splunk: SPLUNK,
  splunk_hec: SPLUNK_HEC,
  sentinel: SENTINEL,
  qradar: QRADAR,
  datadog: DATADOG,
  jira: JIRA,
  pagerduty: PAGERDUTY,
  opsgenie: OPSGENIE,
  servicenow: SERVICENOW,
  generic: GENERIC,
};

export const GUIDE_KINDS: WebhookKind[] = Object.keys(GUIDES) as WebhookKind[];
