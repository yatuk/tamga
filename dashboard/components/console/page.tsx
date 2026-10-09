"use client";

import { useState, type CSSProperties, type ReactNode } from "react";
import { Button, ContentSwitcher, IconButton, InlineNotification, SkeletonText, Switch } from "@carbon/react";
import { TrashCan } from "@carbon/icons-react";
import type { TimeRange } from "@/lib/types";
import { Bar } from "./motion";

/** The page title, one sentence on what the page answers, and its controls. */
export function PageHead({ title, lede, children }: { title: string; lede: ReactNode; children?: ReactNode }) {
  return (
    <div className="t-pagehead">
      <div>
        <h1>{title}</h1>
        <p className="t-lede">{lede}</p>
      </div>
      {children ? <div className="t-controls">{children}</div> : null}
    </div>
  );
}

const RANGES = ["24h", "7d", "30d"] as const;

export function RangeSwitch({ value, onChange }: { value: TimeRange; onChange: (range: TimeRange) => void }) {
  return (
    <ContentSwitcher
      size="sm"
      selectedIndex={Math.max(0, RANGES.indexOf(value as (typeof RANGES)[number]))}
      onChange={({ index }) => onChange(RANGES[index ?? 1])}
      style={{ width: "19rem", flexShrink: 0 }}
    >
      <Switch name="24h" text="24 hours" />
      <Switch name="7d" text="7 days" />
      <Switch name="30d" text="30 days" />
    </ContentSwitcher>
  );
}

/** Why the page has no data, when it has none. Renders nothing otherwise. */
export function DataNotice({ adminKey, failed }: { adminKey: string; failed?: boolean }) {
  if (adminKey && !failed) return null;
  return (
    <InlineNotification
      kind={adminKey ? "error" : "info"}
      lowContrast
      hideCloseButton
      title={adminKey ? "The proxy did not answer." : "No admin key in this browser."}
      subtitle={
        adminKey
          ? "Check that it is running and that the admin key is the right one."
          : "Add one under Settings to read the proxy's data."
      }
      style={{ marginTop: "1.5rem", maxWidth: "none" }}
    />
  );
}

export function Section({
  id,
  title,
  note,
  children,
  style,
}: {
  id: string;
  title: string;
  note?: ReactNode;
  children: ReactNode;
  style?: CSSProperties;
}) {
  return (
    <section aria-labelledby={id} style={style}>
      <div className="t-sectionhead">
        <h2 id={id}>{title}</h2>
        {note ? <span className="t-note">{note}</span> : null}
      </div>
      {children}
    </section>
  );
}

/**
 * The one figure a page is about, said as a sentence, with the supporting
 * facts as a plain list beside it.
 */
export function Statement({
  loading,
  figure,
  quiet = true,
  sentence,
  rest,
  below,
  facts,
}: {
  loading?: boolean;
  figure: ReactNode;
  /** Ink instead of the stamp red. Red is only for "stopped". */
  quiet?: boolean;
  sentence: ReactNode;
  rest?: ReactNode;
  below?: ReactNode;
  facts: [label: string, value: ReactNode][];
}) {
  return (
    <div className="t-statement">
      <div>
        {loading ? (
          <SkeletonText heading width="60%" />
        ) : (
          <>
            <div className="t-figureline">
              <span className="t-figure" data-quiet={quiet}>
                {figure}
              </span>
              <p>{sentence}</p>
            </div>
            {rest ? <p className="t-rest">{rest}</p> : null}
            {below}
          </>
        )}
      </div>
      <dl className="t-facts">
        {facts.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

export interface ShareRow {
  label: string;
  note?: string;
  value: number;
  /** Shown in place of the formatted value, for a row that has none. */
  text?: string;
  data?: boolean;
}

/** Rows with their share drawn in the row itself. */
export function Shares({
  rows,
  head,
  format = (n) => n.toLocaleString("en"),
  scale = "max",
  empty,
}: {
  rows: ShareRow[];
  head: [label: string, value: string] | [label: string, note: string, value: string];
  format?: (n: number) => string;
  /** Bars relative to the largest row, or to the sum of all rows. */
  scale?: "max" | "sum";
  empty: string;
}) {
  if (rows.length === 0) return <p className="t-empty">{empty}</p>;
  const noted = head.length === 3;
  const top = scale === "max" ? Math.max(...rows.map((r) => r.value)) : rows.reduce((sum, r) => sum + r.value, 0);
  return (
    <table className="t-kinds">
      <thead>
        <tr>
          <th>{head[0]}</th>
          {noted ? <th>{head[1]}</th> : null}
          <th className="t-num">{head[head.length - 1]}</th>
          <th className="t-bar">
            <span className="cds--visually-hidden">Share</span>
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={r.label}>
            <td className={r.data ? "t-data" : undefined}>{r.label}</td>
            {noted ? <td className="t-note">{r.note}</td> : null}
            <td className="t-num">{r.text ?? format(r.value)}</td>
            <td className="t-bar">
              <Bar share={top > 0 ? r.value / top : 0} delay={0.15 + i * 0.035} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/**
 * Delete, asked twice in place: the bin turns into the question. No dialog
 * to read for something this small.
 */
export function Remove({
  label,
  verb = "Delete",
  text,
  onConfirm,
  disabled,
}: {
  label: string;
  verb?: string;
  /** A worded button in place of the bin, for something other than deleting a row. */
  text?: string;
  onConfirm: () => void;
  disabled?: boolean;
}) {
  const [asking, setAsking] = useState(false);
  if (!asking && text) {
    return (
      <Button kind="ghost" size="sm" disabled={disabled} onClick={() => setAsking(true)}>
        {text}
      </Button>
    );
  }
  if (!asking) {
    return (
      <IconButton kind="ghost" size="sm" label={label} align="left" disabled={disabled} onClick={() => setAsking(true)}>
        <TrashCan />
      </IconButton>
    );
  }
  return (
    <span className="t-confirm">
      <Button
        kind="danger"
        size="sm"
        autoFocus
        onClick={() => {
          setAsking(false);
          onConfirm();
        }}
      >
        {verb}
      </Button>
      <Button kind="ghost" size="sm" onClick={() => setAsking(false)}>
        Keep
      </Button>
    </span>
  );
}
