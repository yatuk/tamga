import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { policyMock } from "@/__mocks__/policy-handlers";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { ApiError } from "@/lib/api/fetch-core";
import { POLICY_DRAFT_STORAGE } from "./_constants";
import PoliciesPage from "./page";
import { maskedNotifyRules } from "./policyUtils";
import { explainRefusal, sectionWords } from "./refusal";

// CodeMirror needs a real layout. The draft comes from this browser's storage, as it does after a reload.
vi.mock("./policy-code-editor", () => ({ default: () => null }));

// Carbon's tabs construct a ResizeObserver; the stand-in in vitest.setup.ts cannot be called with `new`.
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);

// See __mocks__/policy-handlers.ts for what each key stands for.
const ADMIN_KEY = "test-key";
const ANALYST_KEY = "analyst-key";

const DUAL_CONTROL = { governance: { require_dual_control: true } };
const STOP_PII = { rules: { ...(policyMock.policy.rules as object), pii_detection: { action: "block", severity: "high", notify: ["slack-security"] } } };

function show({ key = ADMIN_KEY, draft }: { key?: string; draft?: string } = {}) {
  localStorage.setItem(ADMIN_KEY_STORAGE, key);
  if (draft) localStorage.setItem(POLICY_DRAFT_STORAGE, draft);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(createElement(QueryClientProvider, { client: qc }, createElement(PoliciesPage)));
  return userEvent.setup();
}

const saveButton = () => screen.findByRole("button", { name: "Save and apply" });

async function save(user: ReturnType<typeof userEvent.setup>) {
  const button = await saveButton();
  await waitFor(() => expect(button).toBeEnabled());
  await user.click(button);
}

async function openTab(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(await screen.findByRole("tab", { name }));
}

beforeEach(() => {
  localStorage.clear();
  policyMock.reset();
});

describe("sectionWords", () => {
  it("names a rule's notify list as its alert destinations", () => {
    expect(sectionWords("rules.pii_detection.notify")).toBe("Alert destinations of rule pii_detection");
  });

  it("names the sections the proxy keeps for admins, and reads out one it does not know", () => {
    expect(sectionWords("rate_limit")).toBe("Rate limits");
    expect(sectionWords("body_limits")).toBe("Body limits");
    expect(sectionWords("some_new_section")).toBe("Some new section");
  });
});

describe("explainRefusal", () => {
  it("says which status a settled proposal has", () => {
    const refusal = explainRefusal(new ApiError("already approved", 409, { code: "proposal_closed", status: "approved" }), "approve");
    expect(refusal.title).toBe("This proposal was already approved.");
  });

  it("keeps the proxy's sentence for a failure without a code", () => {
    const refusal = explainRefusal(new ApiError("entity with this name already exists", 409, { error: "entity with this name already exists" }), "entity-add");
    expect(refusal.title).toBe("The entity could not be added.");
    expect(refusal.detail).toBe("entity with this name already exists");
  });

  it("says a key without the role may not do this", () => {
    const refusal = explainRefusal(new ApiError("role viewer lacks policies:write", 403, {}), "save");
    expect(refusal.title).toBe("The key in use may not do this.");
    expect(refusal.detail).toContain("role viewer lacks policies:write");
  });

  it("says the proxy did not answer when nothing came back", () => {
    expect(explainRefusal(new Error(""), "save").detail).toBe("The proxy did not answer.");
  });
});

describe("Policies page: saving", () => {
  it("names the paths a save changed", async () => {
    const user = show({ draft: policyMock.draft(STOP_PII) });
    await save(user);

    expect(await screen.findByText("Policy saved. The proxy is now enforcing it.")).toBeInTheDocument();
    expect(screen.getByText("One part of the policy changed:")).toBeInTheDocument();
    const changed = screen.getByRole("list", { name: "What changed" });
    expect(within(changed).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["rules.pii_detection.action"]);
    // The draft is the running policy again.
    await waitFor(() => expect(screen.queryByText("Unsaved changes")).toBeNull());
  });

  it("tells a key that is not an admin which sections it may not change, and that nothing was applied", async () => {
    const before = policyMock.policy;
    const draft = policyMock.draft({
      providers: { allowed: ["anthropic"] },
      rules: { ...(before.rules as object), pii_detection: { action: "block", severity: "high", notify: [] } },
    });
    const user = show({ key: ANALYST_KEY, draft });
    await save(user);

    expect(
      await screen.findByText("The key in use is not an admin key, and this change touches parts only an admin may change."),
    ).toBeInTheDocument();
    expect(screen.getByText("Providers")).toBeInTheDocument();
    expect(screen.getByText("providers")).toBeInTheDocument();
    expect(screen.getByText("Alert destinations of rule pii_detection")).toBeInTheDocument();
    expect(screen.getByText(/Nothing was saved, including the parts this key may change\./)).toBeInTheDocument();
    // The rule's action was one the analyst may change. It was not applied either.
    expect(policyMock.policy).toEqual(before);
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
  });

  it("lets a key that is not an admin change what it may", async () => {
    const user = show({ key: ANALYST_KEY, draft: policyMock.draft(STOP_PII) });
    await save(user);
    expect(await screen.findByText("Policy saved. The proxy is now enforcing it.")).toBeInTheDocument();
  });

  it("lists every setting the proxy refused under the path of its field", async () => {
    const draft = policyMock.draft({
      rate_limit: { requests_per_minute: -5 },
      rules: { ...(policyMock.policy.rules as object), prompt_injection: { action: "explode", severity: "medium" } },
    });
    const user = show({ draft });
    await save(user);

    expect(await screen.findByText("2 settings in the policy are not allowed.")).toBeInTheDocument();
    expect(screen.getByText("Nothing was saved. Fix them in the document and try again.")).toBeInTheDocument();
    expect(screen.getByText("rate_limit.requests_per_minute")).toBeInTheDocument();
    expect(screen.getByText("must not be negative")).toBeInTheDocument();
    expect(screen.getByText("rules.prompt_injection.action")).toBeInTheDocument();
    expect(screen.getByText('unknown action "explode"')).toBeInTheDocument();
  });

  it("shows the proxy's remarks on a policy it accepted", async () => {
    const draft = policyMock.draft({ rules: { pii_detection: (policyMock.policy.rules as Record<string, unknown>).pii_detection } });
    const user = show({ draft });
    await save(user);

    expect(await screen.findByText("Policy saved. The proxy is now enforcing it.")).toBeInTheDocument();
    expect(screen.getByText("The proxy has one remark on it:")).toBeInTheDocument();
    expect(screen.getByText("no rule covers prompt injection")).toBeInTheDocument();
  });

  it("says the policy file is read-only when the proxy finds that out on the write", async () => {
    const user = show({ draft: policyMock.draft(STOP_PII) });
    const button = await saveButton();
    await waitFor(() => expect(button).toBeEnabled());
    policyMock.setReadOnly(true);
    await user.click(button);

    expect(await screen.findByText("Nothing was saved. Edit the file there and choose Reload from disk, or mount it writable.")).toBeInTheDocument();
    expect(screen.getByText("The policy file is read-only where the proxy runs.")).toBeInTheDocument();
  });

  it("offers to propose the draft when the proxy turns out to require a second person", async () => {
    const user = show({ draft: policyMock.draft(STOP_PII) });
    const button = await saveButton();
    await waitFor(() => expect(button).toBeEnabled());
    // Someone switched dual control on after this console read the policy.
    policyMock.writeBehind(DUAL_CONTROL);
    await user.click(button);

    expect(await screen.findByText("This proxy requires a second person for every policy change.")).toBeInTheDocument();
    expect(screen.getByText("Nothing was saved. Propose the draft instead, and someone else approves it.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Propose this change" }));
    expect(await screen.findByRole("heading", { name: "Propose the draft" })).toBeInTheDocument();
  });
});

describe("Policies page: proposals", () => {
  it("under dual control proposes the draft, and says why its author cannot approve it", async () => {
    policyMock.reset({ policy: DUAL_CONTROL });
    const user = show({ draft: policyMock.draft(STOP_PII) });

    // Said before anything is tried, and there is no direct save.
    expect(await screen.findByText("This proxy requires a second person for every policy change.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save and apply" })).toBeNull();
    const propose = screen.getByRole("button", { name: "Propose this change" });
    await waitFor(() => expect(propose).toBeEnabled());
    await user.click(propose);

    // A proposal without a reason is not sent.
    const form = (await screen.findByLabelText("What it changes and why")).closest("form") as HTMLFormElement;
    await user.click(within(form).getByRole("button", { name: "Propose this change" }));
    expect(await screen.findByText(/Say what this changes and why\./)).toBeInTheDocument();

    await user.type(screen.getByLabelText("What it changes and why"), "Stop personal data");
    await user.click(within(form).getByRole("button", { name: "Propose this change" }));
    expect(await screen.findByText("Proposal opened.")).toBeInTheDocument();

    // It is listed with who the proxy verified as its author.
    expect(await screen.findByText("Stop personal data")).toBeInTheDocument();
    expect(screen.getByText(/Proposed by the shared admin key/)).toBeInTheDocument();
    expect(screen.getByText(/What it would change: 1 line added, 1 removed/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Approve and apply" }));
    expect(await screen.findByText("You cannot approve this proposal.")).toBeInTheDocument();
    expect(screen.getByText(/The shared admin key counts as one person/)).toBeInTheDocument();
    expect((policyMock.policy.rules as Record<string, { action: string }>).pii_detection.action).toBe("redact");
  });

  it("names the paths an approved proposal changed", async () => {
    policyMock.reset({ policy: DUAL_CONTROL });
    policyMock.addProposal({ author: "apikey:personal-ayse", yaml: policyMock.draft(STOP_PII), message: "Stop personal data" });
    const user = show({ key: "personal-mehmet" });
    await openTab(user, "Proposals");

    expect(await screen.findByText(/Proposed by API key personal-ayse/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Approve and apply" }));

    expect(await screen.findByText("Proposal approved. The proxy is now enforcing it.")).toBeInTheDocument();
    const changed = screen.getByRole("list", { name: "What changed" });
    expect(within(changed).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["rules.pii_detection.action"]);
    await waitFor(() => expect(screen.queryByText("Stop personal data")).toBeNull());
    expect((policyMock.policy.rules as Record<string, { action: string }>).pii_detection.action).toBe("block");
  });

  it("says a proposal was already settled by someone else", async () => {
    const proposal = policyMock.addProposal({ author: "apikey:personal-ayse", yaml: policyMock.draft(STOP_PII), message: "Stop personal data" });
    const user = show();
    await openTab(user, "Proposals");
    const approve = await screen.findByRole("button", { name: "Approve and apply" });
    policyMock.settle(proposal.id, "rejected");
    await user.click(approve);

    expect(await screen.findByText("This proposal was already rejected.")).toBeInTheDocument();
    // Nothing is left to decide on it.
    await waitFor(() => expect(screen.queryByRole("button", { name: "Approve and apply" })).toBeNull());
  });

  it("says the policy changed after a proposal was written", async () => {
    policyMock.addProposal({ author: "apikey:personal-ayse", yaml: policyMock.draft(STOP_PII), message: "Stop personal data" });
    const user = show();
    await openTab(user, "Proposals");
    const approve = await screen.findByRole("button", { name: "Approve and apply" });
    policyMock.writeBehind({ rate_limit: { requests_per_minute: 300 } });
    await user.click(approve);

    expect(await screen.findByText("The policy changed after this proposal was written.")).toBeInTheDocument();
    expect(screen.getByText(/Reject it and propose the change again from the current policy\./)).toBeInTheDocument();
  });

  it("tells a key that is not an admin why it cannot approve a change to an admin section", async () => {
    policyMock.addProposal({ author: "apikey:personal-ayse", yaml: policyMock.draft({ providers: { allowed: ["anthropic"] } }), message: "One provider" });
    const user = show({ key: ANALYST_KEY });
    await openTab(user, "Proposals");
    await user.click(await screen.findByRole("button", { name: "Approve and apply" }));

    expect(
      await screen.findByText("The key in use is not an admin key, and this change touches parts only an admin may change."),
    ).toBeInTheDocument();
    expect(screen.getByText("The proposal was not applied and is still open. Use an admin key under Settings.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Approve and apply" })).toBeInTheDocument();
  });
});

describe("Policies page: masked alert addresses", () => {
  const SECRET = "https://hooks.slack.com/services/T000/B000/s3cr3t";
  const MASKED = "https://hooks.slack.com/***masked***";
  const rulesWith = (webhook: string, action = "redact") => ({
    rules: {
      ...(policyMock.policy.rules as object),
      pii_detection: { action, severity: "high", notify: [{ channel: "slack", webhook }] },
    },
  });
  const UP_FRONT = "This key cannot see where alerts are sent.";

  it("finds the rules whose address is masked, and no others", () => {
    expect(maskedNotifyRules(JSON.stringify(rulesWith(MASKED)))).toEqual(["rules.pii_detection.notify"]);
    expect(maskedNotifyRules(JSON.stringify(rulesWith(SECRET)))).toEqual([]);
    expect(maskedNotifyRules(JSON.stringify(rulesWith("${SLACK_WEBHOOK_URL}")))).toEqual([]);
    // The marker anywhere else is not an address the proxy refuses.
    expect(maskedNotifyRules(JSON.stringify({ name: "***masked***", rules: {} }))).toEqual([]);
    expect(maskedNotifyRules("rules: ***masked***")).toEqual([]);
  });

  it("explains the proxy's refusal of a masked address, naming the rules", () => {
    const refusal = explainRefusal(
      new ApiError("the policy holds a masked webhook URL", 422, { code: "masked_value", fields: ["rules.pii_detection.notify"] }),
      "save",
    );
    expect(refusal.title).toBe("The policy holds a masked alert address, which cannot be saved back.");
    expect(refusal.detail).toMatch(/^Nothing was saved\. With an admin key, write each address in full or, better, as a reference/);
    expect(refusal.sections).toEqual([{ words: "Alert destinations of rule pii_detection", path: "rules.pii_detection.notify" }]);
  });

  it("tells a key below admin up front that it cannot see or save the addresses, and switches the save off", async () => {
    policyMock.reset({ policy: rulesWith(SECRET) });
    // An edit this key may make, in the document as it reads it.
    const user = show({ key: ANALYST_KEY, draft: policyMock.draft(rulesWith(MASKED, "block")) });

    expect(await screen.findByText(UP_FRONT)).toBeInTheDocument();
    expect(screen.getByText(/A masked address cannot be saved back, so the proxy refuses a save or a proposal of this document, and Save and apply is off\./)).toBeInTheDocument();
    expect(screen.getByText(/as a reference such as \$\{SLACK_WEBHOOK_URL\}/)).toBeInTheDocument();
    const named = screen.getByRole("list", { name: "Rules with a masked alert address" });
    expect(within(named).getByText("rules.pii_detection.notify")).toBeInTheDocument();

    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    expect(await saveButton()).toBeDisabled();
    // Nor can the draft be proposed, and the form says why.
    await openTab(user, "Proposals");
    const form = (await screen.findByLabelText("What it changes and why")).closest("form") as HTMLFormElement;
    expect(within(form).getByRole("button", { name: "Propose this change" })).toBeDisabled();
    expect(within(form).getByText(/The draft holds a masked alert address, which the proxy does not take back/)).toBeInTheDocument();
    expect((policyMock.policy.rules as Record<string, { action: string }>).pii_detection.action).toBe("redact");
  });

  it("does not show the masked address as a change the person made", async () => {
    policyMock.reset({ policy: rulesWith(SECRET) });
    const user = show({ key: ANALYST_KEY });
    expect(await screen.findByText(UP_FRONT)).toBeInTheDocument();
    expect(screen.queryByText("Unsaved changes")).toBeNull();
    await openTab(user, "Changes");
    expect(await screen.findByText("The draft is the same as the policy the proxy is running.")).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("s3cr3t");
  });

  it("says a difference in a masked line comes from the key, not from an edit", async () => {
    policyMock.reset({ policy: rulesWith(SECRET) });
    // A draft written with an admin key, read later with a key below admin.
    const user = show({ key: ANALYST_KEY, draft: policyMock.draft({}) });
    expect(await screen.findByText(UP_FRONT)).toBeInTheDocument();
    await openTab(user, "Changes");
    expect(await screen.findByText(/It differs because the draft\s+and the running policy were read with different keys, not because of an edit\./)).toBeInTheDocument();
  });

  it("shows an admin key the addresses, and says nothing", async () => {
    policyMock.reset({ policy: rulesWith(SECRET) });
    show({ draft: policyMock.draft(rulesWith(SECRET, "block")) });
    const button = await saveButton();
    await waitFor(() => expect(button).toBeEnabled());
    expect(screen.queryByText(UP_FRONT)).toBeNull();
    expect(screen.queryByText("The draft holds a masked alert address.")).toBeNull();
  });

  it("never masks a reference, so a key below admin saves as before", async () => {
    policyMock.reset({ policy: rulesWith("${SLACK_WEBHOOK_URL}") });
    const user = show({ key: ANALYST_KEY, draft: policyMock.draft(rulesWith("${SLACK_WEBHOOK_URL}", "block")) });
    await save(user);
    expect(await screen.findByText("Policy saved. The proxy is now enforcing it.")).toBeInTheDocument();
    expect(screen.queryByText(UP_FRONT)).toBeNull();
  });

  it("stops an admin key from saving a draft that was read masked", async () => {
    policyMock.reset({ policy: rulesWith(SECRET) });
    show({ draft: policyMock.draft(rulesWith(MASKED, "block")) });
    expect(await screen.findByText("The draft holds a masked alert address.")).toBeInTheDocument();
    expect(screen.getByText(/It was read with a key that is not an admin key/)).toBeInTheDocument();
    expect(await saveButton()).toBeDisabled();
  });

  it("explains the refusal where a proposal that holds the mask is approved", async () => {
    policyMock.reset({ policy: rulesWith(SECRET) });
    policyMock.addProposal({ author: "apikey:analyst-ayse", yaml: policyMock.draft(rulesWith(MASKED, "block")), message: "Stop personal data" });
    const user = show();
    await openTab(user, "Proposals");
    await user.click(await screen.findByRole("button", { name: "Approve and apply" }));

    expect(await screen.findByText("The policy holds a masked alert address, which cannot be saved back.")).toBeInTheDocument();
    expect(screen.getByText(/^The proposal was not applied and is still open\. With an admin key, write each address in full/)).toBeInTheDocument();
    expect(screen.getByText("Alert destinations of rule pii_detection")).toBeInTheDocument();
    expect((policyMock.policy.rules as Record<string, { action: string }>).pii_detection.action).toBe("redact");
  });
});

describe("Policies page: custom entities", () => {
  it("under dual control says entities are changed through a proposal", async () => {
    policyMock.reset({ policy: DUAL_CONTROL });
    const user = show();
    await openTab(user, "Custom entities");

    expect(await screen.findByText(/entities are not added or deleted here/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add entity" })).toBeDisabled();
  });

  it("says why an entity was not deleted when the proxy requires a second person", async () => {
    const user = show();
    await openTab(user, "Custom entities");
    await user.click(await screen.findByRole("button", { name: "Delete project_mercury" }));
    policyMock.writeBehind(DUAL_CONTROL);
    await user.click(screen.getByRole("button", { name: "Delete the entity" }));

    expect(await screen.findByText("This proxy requires a second person for every policy change.")).toBeInTheDocument();
    expect(screen.getByText(/The entity was not deleted\. Make the change to custom_entities/)).toBeInTheDocument();
    expect(policyMock.policy.custom_entities).toHaveLength(1);
  });

  it("says the file is read-only where an entity was added", async () => {
    const user = show();
    await openTab(user, "Custom entities");
    await user.type(await screen.findByLabelText("Name"), "customer_code");
    await user.type(screen.getByLabelText("Regular expression"), "CUST-.+");
    policyMock.setReadOnly(true);
    await user.click(screen.getByRole("button", { name: "Add entity" }));

    expect(await screen.findByText("The entity was not added. Edit the file there and choose Reload from disk, or mount it writable.")).toBeInTheDocument();
  });
});
