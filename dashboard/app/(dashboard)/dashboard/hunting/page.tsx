"use client";

import { useEffect, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Button, Checkbox, DataTableSkeleton, Search, Select, SelectItem, TextInput } from "@carbon/react";
import { Renew } from "@carbon/icons-react";
import { EventTable, Pager, useOpenEvent } from "@/components/console/events";
import { EXPRESSIVE, PRODUCTIVE } from "@/components/console/motion";
import { ACTION_WORDS, counted, dayLabel, RANGE_WORDS, typeName } from "@/components/console/names";
import { DataNotice, PageHead, RangeSwitch, Remove, Section } from "@/components/console/page";
import { PAGE_SIZE } from "./_constants";
import type { SavedHunt } from "./_types";
import { useHuntingPage } from "./useHuntingPage";

const ACTIONS = ["BLOCK", "STRIP", "REDACT", "WARN", "PASS"];
const SEVERITIES = ["critical", "high", "medium", "low"];
const TYPES = ["pii", "secret", "injection", "custom"];

export default function HuntingPage() {
  const m = useHuntingPage();
  const events = m.data?.events ?? [];
  const total = m.data?.total ?? 0;
  const { openId, open, missing, toggle, close } = useOpenEvent(events, m.adminKey);
  const f = m.filters;
  // A saved hunt may carry a type this list does not know.
  const types = f.finding_type && !TYPES.includes(f.finding_type) ? [...TYPES, f.finding_type] : TYPES;

  return (
    <>
      <PageHead
        title="Threat hunting"
        lede={`Search the requests of ${RANGE_WORDS[m.range]} by what was done to them, what was found in them, or the text that matched.`}
      >
        <RangeSwitch value={m.range} onChange={m.setRange} />
        <Button kind="tertiary" size="sm" renderIcon={Renew} onClick={() => void m.refetch()} disabled={m.isFetching || !m.adminKey}>
          Search again
        </Button>
      </PageHead>

      <DataNotice adminKey={m.adminKey} failed={!!m.error} />

      <div className="t-hunt">
        <div>
          <Search
            size="lg"
            labelText="Search"
            placeholder="A request ID, or text a finding matched"
            value={f.q}
            onChange={(e) => m.setFilter("q", e.target.value)}
          />
          <div className="t-fields">
            <Select id="hunt-action" labelText="What was done" value={f.action} onChange={(e) => m.setFilter("action", e.target.value)}>
              <SelectItem value="" text="Anything" />
              {ACTIONS.map((a) => (
                <SelectItem key={a} value={a} text={ACTION_WORDS[a] ?? a} />
              ))}
            </Select>
            <Select id="hunt-type" labelText="Kind of finding" value={f.finding_type} onChange={(e) => m.setFilter("finding_type", e.target.value)}>
              <SelectItem value="" text="Any kind" />
              {types.map((t) => (
                <SelectItem key={t} value={t} text={typeName(t)} />
              ))}
            </Select>
            <Select id="hunt-severity" labelText="Severity" value={f.severity} onChange={(e) => m.setFilter("severity", e.target.value)}>
              <SelectItem value="" text="Any severity" />
              {SEVERITIES.map((s) => (
                <SelectItem key={s} value={s} text={s[0].toUpperCase() + s.slice(1)} />
              ))}
            </Select>
            <TextInput
              id="hunt-category"
              labelText="Finding name contains"
              placeholder="credit_card, jailbreak"
              autoComplete="off"
              spellCheck={false}
              value={f.category}
              onChange={(e) => m.setFilter("category", e.target.value)}
            />
            <TextInput
              id="hunt-technique"
              labelText="Technique or OWASP code"
              placeholder="LLM01"
              autoComplete="off"
              spellCheck={false}
              value={f.technique}
              onChange={(e) => m.setFilter("technique", e.target.value)}
            />
            <TextInput
              id="hunt-provider"
              labelText="Sent to"
              placeholder="openai, anthropic"
              autoComplete="off"
              spellCheck={false}
              disabled={m.shadow}
              value={f.provider}
              onChange={(e) => m.setFilter("provider", e.target.value)}
            />
          </div>
          <div className="t-formrow">
            <Checkbox
              id="hunt-shadow"
              labelText="Only requests to a provider that is not in the routing table"
              checked={m.shadow}
              onChange={(_, { checked }) => m.setShadow(checked)}
            />
            {m.activeFilterCount > 0 ? (
              <Button kind="ghost" size="sm" onClick={m.clearFilters} style={{ marginLeft: "auto" }}>
                Clear {counted(m.activeFilterCount, "filter")}
              </Button>
            ) : null}
          </div>
        </div>

        <SavedHunts hunts={m.savedHunts} onApply={m.applyHunt} onDelete={m.deleteHunt} onSave={m.saveHunt} />
      </div>

      {/* Without a key or an answer there are no matches to count. */}
      {!m.adminKey || (m.error && !m.data) ? null : (
        <Section
          id="matches"
          title="Matches"
          note={m.isLoading ? "Searching" : counted(total, "request")}
          style={{ marginTop: "3rem" }}
        >
          {m.isLoading ? (
            <DataTableSkeleton columnCount={6} rowCount={8} showHeader={false} showToolbar={false} />
          ) : (
            <EventTable
              label="Matches"
              events={events}
              swapKey={`${JSON.stringify(f)}|${m.shadow}|${m.range}|${m.page}`}
              openId={openId}
              open={open}
              missing={missing}
              onToggle={toggle}
              onClose={close}
              adminKey={m.adminKey}
              empty={
                m.activeFilterCount > 0
                  ? "No request in this window fits the search. Loosen a filter or pick a longer range."
                  : "No request went through in this window."
              }
              after={<Pager page={m.page} pages={Math.max(1, Math.ceil(total / PAGE_SIZE))} onPage={m.setPage} />}
            />
          )}
        </Section>
      )}
    </>
  );
}

function SavedHunts({
  hunts,
  onApply,
  onDelete,
  onSave,
}: {
  hunts: SavedHunt[];
  onApply: (hunt: SavedHunt) => void;
  onDelete: (id: string) => Promise<void>;
  onSave: (name: string) => Promise<void>;
}) {
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState("");
  const save = useRef<HTMLButtonElement>(null);
  const named = useRef(false);
  // Leaving the name field, saved or not, puts the focus back on the button that opened it.
  useEffect(() => {
    if (naming) named.current = true;
    else if (named.current) {
      named.current = false;
      save.current?.focus();
    }
  }, [naming]);

  return (
    <section aria-labelledby="saved">
      <div className="t-sectionhead">
        <h2 id="saved">Saved hunts</h2>
        {naming ? null : (
          <Button ref={save} kind="ghost" size="sm" onClick={() => setNaming(true)}>
            Save this search
          </Button>
        )}
      </div>

      {/* The name field opens in place, where the saved hunt will then appear. */}
      <AnimatePresence initial={false}>
        {naming ? (
          <motion.form
            key="naming"
            className="t-savehunt"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.24, ease: EXPRESSIVE }}
            onSubmit={(e) => {
              e.preventDefault();
              if (!name.trim()) return;
              // A refused save is reported by the page; the field stays open with the name in it.
              onSave(name).then(
                () => {
                  setName("");
                  setNaming(false);
                },
                () => undefined,
              );
            }}
          >
            <TextInput
              id="hunt-name"
              labelText="Name for the current search"
              placeholder="Leaked cloud keys"
              autoComplete="off"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === "Escape" && setNaming(false)}
            />
            <Button type="submit" kind="primary" size="md" disabled={!name.trim()}>
              Save search
            </Button>
          </motion.form>
        ) : null}
      </AnimatePresence>

      {hunts.length === 0 ? (
        <p className="t-empty" style={{ padding: "1.5rem 0" }}>
          None yet. Build a search and save it to run it again later.
        </p>
      ) : (
        <ul className="t-hunts">
          <AnimatePresence initial={false}>
            {hunts.map((h) => (
              <motion.li
                key={h.id}
                layout
                initial={{ opacity: 0, y: -6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0 }}
                transition={{ duration: 0.2, ease: PRODUCTIVE }}
              >
                <button type="button" className="t-huntname" onClick={() => onApply(h)}>
                  <b>{h.name}</b>
                  <span className="t-note">Saved {dayLabel(h.updated_at)}</span>
                </button>
                <Remove label={`Delete ${h.name}`} verb="Delete the search" onConfirm={() => void onDelete(h.id)} />
              </motion.li>
            ))}
          </AnimatePresence>
        </ul>
      )}
    </section>
  );
}
