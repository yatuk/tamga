import NextLink from "next/link";
import { Button } from "@carbon/react";
import { PageHead } from "@/components/console/page";

export default function DashboardNotFound() {
  return (
    <PageHead title="There is no page here" lede="The address does not match a page of the console. It may have been renamed, or mistyped.">
      <Button as={NextLink} href="/dashboard" kind="primary" size="sm">
        Go to the overview
      </Button>
    </PageHead>
  );
}
