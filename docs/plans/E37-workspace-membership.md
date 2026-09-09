# E37 -- Let a workspace admin invite and manage other members

fidelity: executable

Acceptance: A workspace owner or admin generates a single-use, expiring invite link naming a role. The invited person authenticates (existing passkey flow; a new passkey identity if they don't have one) and sees an explicit confirm step naming the workspace before becoming a member -- never auto-joined. Owners/admins can list members, change a member's role, and revoke membership. No email address is collected or required anywhere in this flow.

Grounded against the live repo (`origin/main`, 2026-09-08) before writing this: `organizations` already has the shape of a workspace (agents and verified domains belong to it) via `human_owner_id` -- singular, no membership table. `verified_domains`/`domain_grants` (`internal/api/domains.go`) already implement DNS TXT-record domain verification for agent/posting scope; this epic does not touch or duplicate that. Human identity is passkey-only (`Human{ID,Name,CSRF,Keys}`, WebAuthn) with zero email anywhere in the schema, and `docs/api.md` states this as deliberate ("no email recovery or proof of human/domain ownership in this flow"). dec-1083's original design proposed a matching-domain-email auto-offer trigger; dec-1087 dropped it from scope entirely (not deferred) -- David's actual ask was "let a workspace admin add other users," which invite-links fully satisfy without reversing the no-email stance. An admin explicitly choosing who to invite is a narrower trust boundary than an offer surfacing to anyone typing a matching-domain email, so this fully honors dec-1083's "make it safer" intent, more so than the first draft. `human_owner_id` is left unchanged for backward compatibility; membership is additive, not a migration of existing ownership semantics.

New use cases (P0 unless noted):
- UC-W01 An owner/admin invites another person into the workspace via a single-use, expiring link, choosing their role.
- UC-W02 An invited person sees an explicit confirm step naming the workspace before becoming a member -- never silently enrolled.
- UC-W03 (P1) An owner/admin manages membership after the fact: list members, change role, revoke access.

- [ ] T37.1 Data model and migration: `memberships` and `invites`  Owner: api  Est: 2h  delivers: [memberships(human_id,org_id,role,joined_at), invites(id,org_id,token_digest,role,created_by,expires_at,used_at,revoked_at)]  kind: plan
  Area: new migration (next available workspace-membership migration, allocated at implementation after reconciling unmerged migrations), internal/store. `human_owner_id` stays as-is; membership is additive alongside it, not a replacement. Token stored as a digest, never plaintext, matching the existing domain-challenge-token pattern in `internal/api/domains.go`. Unique constraint on (human_id, org_id).
  acc: [A membership row requires both a valid human_id and org_id; an expired or already-used invite cannot be redeemed twice]
- [ ] T37.2 Implement admin invite creation  Owner: api  Est: 1.5h  verifies: [UC-W01]  blocked-by: [T37.1]
  Area: internal/web (new endpoint, POST /human/workspaces/{id}/invites using the human session and CSRF token; do not infer human owner/admin authority from an agent bearer credential). Only the org owner or a member with an admin role may create an invite. Configurable expiry (default matches the existing domain-challenge window), one-time-use token returned once at creation (never re-displayed), chosen role validated against a fixed allowed set. HTTP tests assert non-admin denial and expiry enforcement.
  acc: [A non-admin credential cannot create an invite, and a created invite's token is never retrievable again after the creation response]
- [ ] T37.3 Implement confirm-to-join redemption  Owner: api/web  Est: 2h  verifies: [UC-W02]  blocked-by: [T37.1]
  Area: internal/web (new invite-landing route), internal/api (redeem endpoint). Visiting a valid invite link while unauthenticated prompts the existing passkey sign-in/registration flow first; once authenticated, shows an explicit page naming the workspace and role with separate Accept/Decline actions -- Accept is the only path that creates a membership row. A decline or an abandoned link leaves no membership and no residual state. Browser journey test covers accept, decline, and an already-used link.
  acc: [Opening a valid invite link never creates a membership until the invitee's own explicit Accept click, and an already-used or expired link shows a clear denial, not a stale confirm screen]
- [ ] T37.4 Implement membership management  Owner: api/web  Est: 1.5h  verifies: [UC-W03]  blocked-by: [T37.1]
  Area: internal/web (workspace dashboard's existing views from PR#52 -- add a Members view alongside Overview/Agents/Installations/Domains/Security), internal/api (list/update-role/revoke endpoints). Only owner/admin roles may change roles or revoke; a member can always leave their own membership. Revoking removes the membership row immediately; it does not touch the human's other org memberships or agent credentials.
  acc: [A non-admin member cannot revoke another member's access, and a revoked member loses workspace access on their next request without affecting their own account elsewhere]
- [ ] T37.5 Deploy and live-verify the complete invite loop  Owner: release  Est: 1.5h  verifies: [UC-W01, UC-W02, UC-W03]  blocked-by: [T37.2, T37.3, T37.4]  lane: agent
  Area: release pipeline and evidence. Merge reviewed functionality, run fresh PostgreSQL race suites and browser journeys, then rolling deployment. Live-verify: create a real invite, redeem it from a second passkey identity with an explicit confirm click, confirm the new member appears in the Members view, revoke it, confirm access is actually removed. Record the live invite/member IDs used and their outcome; no claim of correctness from unit tests alone.
  acc: [A real invite created, redeemed with explicit confirmation, and later revoked produces the expected membership state at every step, verified against the live deployment, not a local test run]

Waves: T37.1 alone; then T37.2 and T37.3 in parallel (two workers, share the migration from T37.1); T37.4 after T37.1; T37.5 after T37.2-T37.4 land.

Numbering reconciliation: E30 and migration 015 are already assigned to the pending durable improvement operations branch. This membership contract uses E37/T37 and allocates its migration at implementation.

Progress: 2026 09 08 -- contract written (chief-marketer, per dec-1085 authorship routing), scope corrected from dec-1083's original domain-auto-offer design per dec-1086/dec-1087 (invite-links-only; no email identity added). Not yet dispatched.
