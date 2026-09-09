# ADR 001: Local agent memory with Serenity and a Go client

## Status
Accepted direction; upstream writer embedding contract proposed, not yet accepted by Serenity.

## Date
2026-09-07

## Context
Agents need durable knowledge across sessions. Ajent currently distributes a Python client, MCP server and hooks, alongside an older Go aj command. Serenity provides file-backed memory, a derived SQLite index and a read-only Go facade. Its public facade cannot create memories, and its single-writer ordering does not extend across processes or machines.

## Decision
Extend the existing Go aj executable with CLI, MCP and local-service modes. Keep the network service in Ajent; keep the memory engine in Serenity. Seek a supported Serenity host API that owns initialization, writer lifecycle and memory operations. Never import another module's internal packages or duplicate its canonical writers. If the public host API is unavailable, a pinned separate Serenity process is an explicit interim packaging alternative, not a claimed single-binary implementation.

Memory is local by default and requires neither Ajent signup nor a verified domain. Store private brain directories under the user's data directory, outside source repositories. Local project bindings select stable brain IDs; repository files cannot grant access to other brains. Multiple sessions may share a brain, but exactly one local service owns its writer and background jobs. No shared-filesystem or cross-machine concurrent writer guarantee is implied.

Agents may read the canonical Markdown as reference data. Controlled changes go through memory tools. Direct edits are detected and reconciled; they never silently overwrite newer canonical state. Local files are not an isolation boundary between programs running as the same OS user. Use separate OS accounts or sandbox permissions when stronger isolation is required.

Start with explicit remember, lexical recall, inspect, correct/supersede and forget operations. No automatic transcript ingestion, external model calls, Git pushes or publication. Optional models require explicit provider configuration. Publishing a selected finding to Ajent is a separate authorized network action.

Free local memory includes export and user-managed Git transport. Managed cross-machine sync, backups/recovery and device management are a potential paid product, gated on demonstrated local usefulness and willingness to pay. No price or launch date is decided. Shared team memory is a later product decision. Subscription expiry must not remove local access or export.

## Consequences
This preserves portable, inspectable memory and avoids a database-only rewrite. It requires upstream work before writable embedding, compatibility tests before retiring Python, and a separate concurrency/security design before paid sync. Git is still a runtime dependency unless initialization/storage changes upstream; one Go binary does not mean dependency-free installation. Forget initially means exclusion from active recall; historical Git objects and backups require a separate purge procedure, disclosed to users.

## Alternatives
Database-only hosted memory was deferred because it requires changing Serenity's canonical storage and creates an online dependency. A brain per session was rejected as the default because it fragments continuity. Unrestricted concurrent Markdown writes and automatic sync of live brain directories were rejected because they bypass ordering and conflict handling.
