"use client";

import { useState } from "react";
import {
  Button,
  Select,
  SelectItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TextInput,
} from "@carbon/react";
import { Remove } from "@/components/console/page";
import type { TeamMember, TeamRole } from "@/lib/api";
import { assignedRole, ROLES, roleSetLabel, sessionsEndedLabel, unassignedWord, type DefaultRole } from "./access";

/** How the last change to a member ended, said at the member's row until the next change to that member. */
export type RowNote = { kind: "done" | "refused"; text: string };

function Note({ note }: { note?: RowNote }) {
  if (!note) return null;
  return note.kind === "refused" ? (
    <span className="t-refused" role="alert">
      {note.text}
    </span>
  ) : (
    <span className="t-rownote" role="status">
      {note.text}
    </span>
  );
}

/**
 * The role is picked first and saved second. A select that saved on change
 * would save every role the arrow keys pass through on the way to the one
 * meant, and each of those saves signs the person out.
 */
function RoleCell({
  member,
  assigned,
  defaultRole,
  pending,
  onSave,
}: {
  member: TeamMember;
  assigned: TeamRole | "";
  defaultRole: DefaultRole;
  pending: boolean;
  onSave: (role: TeamRole) => void;
}) {
  const [draft, setDraft] = useState<TeamRole | "">(assigned);
  const changed = draft !== "" && draft !== assigned;
  // Without a role and without a default there is no session to end.
  const signsOut = assigned !== "" || defaultRole === "viewer";
  return (
    <>
      <span className="t-rolecell">
        <Select
          id={`role-${member.user_id}`}
          size="sm"
          inline
          hideLabel
          labelText={`Role of ${member.name || member.user_id}`}
          value={draft}
          disabled={pending}
          onChange={(e) => setDraft(e.target.value as TeamRole | "")}
        >
          {assigned === "" ? <SelectItem value="" text={unassignedWord(defaultRole)} /> : null}
          {ROLES.map((r) => (
            <SelectItem key={r.role} value={r.role} text={r.name} />
          ))}
        </Select>
        {changed ? (
          <>
            <Button kind="primary" size="sm" disabled={pending} onClick={() => onSave(draft)}>
              Save role
            </Button>
            <Button kind="ghost" size="sm" disabled={pending} onClick={() => setDraft(assigned)}>
              Cancel
            </Button>
          </>
        ) : null}
      </span>
      {changed ? (
        <span className="t-rownote">
          {signsOut
            ? "Saving signs this person out everywhere. They sign in again with the new role."
            : "Saving lets this account sign in."}
        </span>
      ) : null}
    </>
  );
}

/** Every account the proxy lists, with what it may do and the three things an admin does to it. */
export function Members({
  members,
  defaultRole,
  pending,
  notes,
  onRole,
  onRemove,
  onSignOut,
}: {
  members: TeamMember[];
  defaultRole: DefaultRole;
  pending: boolean;
  notes: Record<string, RowNote>;
  onRole: (member: TeamMember, role: TeamRole) => void;
  onRemove: (member: TeamMember) => void;
  onSignOut: (member: TeamMember) => void;
}) {
  return (
    <TableContainer className="t-table">
      <Table size="md" aria-label="Members">
        <TableHead>
          <TableRow>
            <TableHeader>Member</TableHeader>
            <TableHeader>Email</TableHeader>
            <TableHeader>Access</TableHeader>
            <TableHeader>Role set</TableHeader>
            <TableHeader>
              <span className="cds--visually-hidden">Sign out or remove</span>
            </TableHeader>
          </TableRow>
        </TableHead>
        <TableBody>
          {members.map((m) => {
            const assigned = assignedRole(m, defaultRole);
            const who = m.name || m.user_id;
            const ended = sessionsEndedLabel(m);
            return (
              <TableRow key={m.user_id}>
                <TableCell>
                  {m.name || <span className="t-data">{m.user_id}</span>}
                  {m.name ? <span className="t-note t-data t-under">{m.user_id}</span> : null}
                </TableCell>
                <TableCell>{m.email || <span className="t-note">Not known</span>}</TableCell>
                <TableCell>
                  <RoleCell
                    // A saved role is a new starting point for the draft.
                    key={`${m.user_id}|${assigned}`}
                    member={m}
                    assigned={assigned}
                    defaultRole={defaultRole}
                    pending={pending}
                    onSave={(role) => onRole(m, role)}
                  />
                  <Note note={notes[m.user_id]} />
                </TableCell>
                <TableCell>
                  {roleSetLabel(m, defaultRole)}
                  {ended ? <span className="t-note t-under">{ended}</span> : null}
                </TableCell>
                <TableCell>
                  <span className="t-rowactions">
                    {/* Asked twice like the removal, in ink: the person can sign in again. */}
                    <Remove
                      label={`Sign out ${who} everywhere`}
                      text="Sign out everywhere"
                      verb="Sign out"
                      keep="Keep signed in"
                      recoverable
                      disabled={pending}
                      onConfirm={() => onSignOut(m)}
                    />
                    {assigned ? (
                      <Remove
                        label={`Remove access of ${who}`}
                        text="Remove access"
                        verb="Remove access and sign out"
                        disabled={pending}
                        onConfirm={() => onRemove(m)}
                      />
                    ) : null}
                  </span>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

/**
 * A role for an account the list does not show. Without a directory the proxy
 * knows an account only once it holds a role, so the first role is given by id.
 */
export function GiveRole({
  pending,
  note,
  onGive,
}: {
  pending: boolean;
  note?: RowNote;
  onGive: (userId: string, role: TeamRole) => void;
}) {
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<TeamRole>("viewer");
  const [submitted, setSubmitted] = useState(false);
  const idError = userId.trim() ? "" : "Enter the account's user id.";
  return (
    <>
      <form
        noValidate
        className="t-form"
        onSubmit={(e) => {
          e.preventDefault();
          setSubmitted(true);
          if (!idError) onGive(userId.trim(), role);
        }}
      >
        <TextInput
          id="team-user-id"
          labelText="User id"
          helperText="The proxy names it when it refuses the account's sign-in. For GitHub it is the account's number."
          autoComplete="off"
          spellCheck={false}
          className="t-data"
          value={userId}
          onChange={(e) => setUserId(e.target.value)}
          invalid={submitted && !!idError}
          invalidText={idError}
        />
        <Select id="team-new-role" labelText="Role" value={role} onChange={(e) => setRole(e.target.value as TeamRole)}>
          {ROLES.map((r) => (
            <SelectItem key={r.role} value={r.role} text={r.name} />
          ))}
        </Select>
        <div className="t-buttons">
          <Button type="submit" kind="primary" size="md" disabled={pending}>
            Give the role
          </Button>
        </div>
      </form>
      <Note note={note} />
    </>
  );
}
