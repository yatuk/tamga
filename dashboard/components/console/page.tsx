"use client";

import { useEffect, useLayoutEffect, useRef, useState, type ComponentProps, type CSSProperties, type ReactNode } from "react";
import { Button, ContentSwitcher, IconButton, InlineNotification, SkeletonText, Switch, TableContainer } from "@carbon/react";
import { TrashCan } from "@carbon/icons-react";
import { useHealth } from "@/hooks/useHealth";
import { permissionDenied, sessionRefused } from "@/lib/api/fetch-core";
import type { TimeRange } from "@/lib/types";
import { Bar } from "./motion";
import { RefusedKeyNotice } from "./refused-key";

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

/**
 * Why the page has no data, when it has none. Renders nothing otherwise.
 *
 * `error` is what the page's main read threw. When the proxy refused it
 * because the key's role is too low, the notice says so, naming `what` the
 * page lists ("API keys"); the proxy is up and the key is valid in that case.
 */
export function DataNotice({
  adminKey,
  failed,
  error,
  what = "this page's data",
}: {
  adminKey: string;
  failed?: boolean;
  error?: unknown;
  what?: string;
}) {
  // The health check is answered without a key, so it tells a proxy that is
  // down from one that is up and refused the key or could not serve the data.
  const health = useHealth();
  if (adminKey && !failed && !error) return null;
  const denied = adminKey ? permissionDenied(error) : null;
  // A session the proxy no longer honours is not a wrong key, and says so itself.
  const session = adminKey ? sessionRefused(error) : null;
  // Only while it is answering: a key refused before an outage says nothing of the proxy now.
  // Which key was refused, and the way to drop a stored one, is the notice's own business.
  if (adminKey && !denied && !session && health.reading.standing === "rejected") return <RefusedKeyNotice lead="It is up." />;
  const [title, subtitle] = !adminKey
    ? ["No admin key in this browser.", "Add one under Settings to read the proxy's data."]
    : denied
      ? [`This key's role cannot read ${what}.`, `${denied.needs} Use a key with a higher role under Settings.`]
      : session
        ? [session.title, session.detail]
        : health.data && !health.isError
          ? ["The proxy is up, but it did not return this page's data.", "Proxy status shows whether one of its parts is down."]
          : ["The proxy did not answer.", "Check that it is running and that this console points at it."];
  return (
    <InlineNotification
      kind={adminKey ? "error" : "info"}
      lowContrast
      hideCloseButton
      title={title}
      subtitle={subtitle}
      style={{ marginTop: "1.5rem", maxWidth: "none" }}
    />
  );
}

/**
 * A page with nothing to show: its title, what it is for, and why there is
 * nothing. Used in place of the page's body when there is no key, or when the
 * read the page is built on failed with nothing loaded before it, so that
 * empty figures are never read as "nothing happened".
 */
export function NoData({
  title,
  lede,
  adminKey,
  error,
  what,
  children,
}: {
  title: string;
  lede: ReactNode;
  adminKey: string;
  error?: unknown;
  what?: string;
  children?: ReactNode;
}) {
  return (
    <>
      <PageHead title={title} lede={lede}>
        {children}
      </PageHead>
      <DataNotice adminKey={adminKey} failed error={error} what={what} />
    </>
  );
}

/**
 * Where a page's figures will be, while its first read is on its way. A page
 * draws this in place of its figures, never a 0 or "nothing in this window".
 */
export function PageLoading() {
  return (
    <div role="status" aria-label="Loading" style={{ marginTop: "2.5rem" }}>
      <SkeletonText heading width="40%" />
      <SkeletonText paragraph lineCount={6} />
    </div>
  );
}

/**
 * What the proxy said when it refused an action because the key's role is too
 * low, beside the button that asked. The console cannot know a key's role
 * beforehand, so the button is there for everyone and the answer is shown.
 * The same for a session the proxy no longer honours. Renders nothing for any
 * other error: those are reported where they were.
 */
export function Refused({ error }: { error: unknown }) {
  const refused = (permissionDenied(error) ?? sessionRefused(error))?.refused;
  if (!refused) return null;
  return (
    <span className="t-refused" role="alert">
      {refused}
    </span>
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
  stopped,
  sentence,
  rest,
  below,
  facts,
  factsNote,
}: {
  loading?: boolean;
  figure: ReactNode;
  /**
   * The figure counts requests that were stopped, and there are some: it is
   * drawn in the stamp red. Nothing else is; a bad answer that is not a
   * stopped request (the proxy down, a target missed) is said in ink.
   */
  stopped?: boolean;
  sentence: ReactNode;
  rest?: ReactNode;
  below?: ReactNode;
  facts: [label: string, value: ReactNode][];
  /** A line over the facts, for what has to be said of all of them: that they are an earlier answer. */
  factsNote?: ReactNode;
}) {
  const list = (
    <dl className="t-facts">
      {facts.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );
  return (
    <div className="t-statement">
      <div>
        {loading ? (
          <SkeletonText heading width="60%" />
        ) : (
          <>
            <div className="t-figureline">
              <span className="t-figure" data-stopped={stopped ? "true" : undefined}>
                {figure}
              </span>
              <p>{sentence}</p>
            </div>
            {rest ? <p className="t-rest">{rest}</p> : null}
            {below}
          </>
        )}
      </div>
      {/* While the figure is on its way so are the facts: none is drawn from a default. */}
      {loading ? (
        <div />
      ) : factsNote ? (
        <div>
          {factsNote}
          {list}
        </div>
      ) : (
        list
      )}
    </div>
  );
}

export interface ShareRow {
  /** What tells two rows apart, where two could carry the same label. */
  id?: string;
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
        {rows.map((r) => (
          <tr key={r.id ?? r.label}>
            <td className={r.data ? "t-data" : undefined}>{r.label}</td>
            {noted ? <td className="t-note">{r.note}</td> : null}
            <td className="t-num">{r.text ?? format(r.value)}</td>
            <td className="t-bar">
              <Bar share={top > 0 ? r.value / top : 0} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/**
 * Delete, asked twice in place: the bin turns into the question. No dialog
 * to read for something this small. The question opens with the focus on the
 * way out, so a second Enter keeps the thing; Escape does the same.
 *
 * Neither answer is red: red is a stopped request. An answer that cannot be
 * taken back is the solid ink button, and its words name what will be gone
 * ("Delete the pattern", "Revoke the key for good"). One that can be undone
 * says so by passing recoverable, and is the outlined ink button.
 */
export function Remove({
  label,
  verb,
  text,
  keep = "Keep",
  recoverable,
  onConfirm,
  disabled,
}: {
  /** The accessible name of the bin: what it removes, by name ("Delete Leaked keys"). */
  label: string;
  /** The answer that does it. For something that cannot be undone it names what will be gone. */
  verb: string;
  /** The way out, where "Keep" alone would not say what is kept. */
  keep?: string;
  /**
   * The thing can be had back (a session can be signed in to again). The
   * answer is then drawn outlined instead of solid. Leave it out for anything
   * that is gone for good.
   */
  recoverable?: boolean;
  /** A worded button in place of the bin, for something other than deleting a row. */
  text?: string;
  onConfirm: () => void;
  disabled?: boolean;
}) {
  const [asking, setAsking] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const asked = useRef(false);
  // Once the question is answered the focus goes back to where it was asked.
  useEffect(() => {
    if (asking) asked.current = true;
    else if (asked.current) {
      asked.current = false;
      trigger.current?.focus();
    }
  }, [asking]);

  if (!asking && text) {
    return (
      <Button ref={trigger} kind="ghost" size="sm" disabled={disabled} onClick={() => setAsking(true)}>
        {text}
      </Button>
    );
  }
  if (!asking) {
    return (
      <IconButton ref={trigger} kind="ghost" size="sm" label={label} align="left" disabled={disabled} onClick={() => setAsking(true)}>
        <TrashCan />
      </IconButton>
    );
  }
  return (
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <span className="t-confirm" onKeyDown={(e) => e.key === "Escape" && setAsking(false)}>
      <Button
        kind={recoverable ? "tertiary" : "primary"}
        size="sm"
        data-answer={recoverable ? "recoverable" : "irreversible"}
        onClick={() => {
          setAsking(false);
          onConfirm();
        }}
      >
        {verb}
      </Button>
      <Button kind="ghost" size="sm" autoFocus onClick={() => setAsking(false)}>
        {keep}
      </Button>
    </span>
  );
}

/** Which side of a table holds columns that are out of view. Empty when it fits. */
type Sideways = "" | "left" | "right" | "both";

/**
 * A Carbon table in the box it scrolls in. Use it wherever a table would be
 * wrapped in `TableContainer`: it is that, and it takes the same props.
 *
 * A table that is wider than its place on the page scrolls sideways, and a
 * table that is cut at the edge with nothing said looks like a table that
 * ends there. So while columns are out of view a line above the table says
 * that it scrolls, an ink rule stands on the side they are on, and the box
 * that scrolls is a tab stop with a name, so that the arrow keys move it and
 * a screen reader says what it is. A table that fits gets none of the three.
 */
export function ScrollTable({ children, ...rest }: ComponentProps<typeof TableContainer>) {
  const note = useRef<HTMLParagraphElement>(null);
  const [box, setBox] = useState<HTMLElement | null>(null);
  const [more, setMore] = useState<Sideways>("");

  // Carbon draws the box that scrolls and gives no handle on it. It is looked
  // for again whenever the container's content changes, since the table may
  // come later than its container (after a skeleton) or be replaced.
  useLayoutEffect(() => {
    const container = note.current?.parentElement;
    if (!container) return;
    const find = () => setBox(container.querySelector<HTMLElement>(".cds--data-table-content"));
    find();
    const seen = new MutationObserver(find);
    seen.observe(container, { childList: true, subtree: true });
    return () => seen.disconnect();
  }, []);

  // Before paint, so that a table that scrolls is never drawn first without its note.
  useLayoutEffect(() => {
    if (!box) return;
    const read = () => {
      const hidden = box.scrollWidth - box.clientWidth;
      const left = box.scrollLeft > 1;
      const right = box.scrollLeft < hidden - 1;
      setMore(hidden <= 1 ? "" : left && right ? "both" : left ? "left" : "right");
    };
    read();
    box.addEventListener("scroll", read, { passive: true });
    // The window, the panel beside the table, or the rows themselves can change what fits.
    const observer = new ResizeObserver(read);
    observer.observe(box);
    const table = box.querySelector("table");
    if (table) observer.observe(table);
    return () => {
      box.removeEventListener("scroll", read);
      observer.disconnect();
    };
  }, [box]);

  const scrolls = more !== "";
  useLayoutEffect(() => {
    if (!box) return;
    if (!scrolls) {
      for (const name of ["tabindex", "role", "aria-label"]) box.removeAttribute(name);
      return;
    }
    // The table's own name, without the instructions some carry after it.
    const name = box.querySelector("table")?.getAttribute("aria-label")?.split(".")[0];
    box.tabIndex = 0;
    box.setAttribute("role", "region");
    box.setAttribute("aria-label", `${name || "Table"}, scrolls sideways`);
  }, [box, scrolls]);

  return (
    <TableContainer {...rest} data-more={more || undefined}>
      <p ref={note} className="t-scrollnote" hidden={!scrolls}>
        This table is wider than the page. Scroll it sideways to read the other columns.
      </p>
      {children}
    </TableContainer>
  );
}
