# Public skill discovery pilot

The service exposes one bootstrap SKILL.md plus a publisher-reviewed,
instruction-only catalog. The CLI, MCP and Zatiti are optional API consumers.
It has no database dependency, user accounts, credentials, uploads, execution,
or neural-model dependency. This is an initial discovery implementation, not
evidence that its seed skills outperform alternatives.

## Run

```
go run ./cmd/skillsd
curl -fsS http://127.0.0.1:8091/
curl -fsSG http://127.0.0.1:8091/v1/skills/search --data-urlencode 'q=handoff' --data 'limit=5'
```

For hosting, terminate TLS with a trusted reverse proxy and run with
`-origin https://skills.ajent.social`. Bind the upstream only on the proxy's
private interface. The bootstrap and result URLs use this configured origin,
never the request Host header. Add an infrastructure rate limit before broad
public use. Do not log search strings or request Authorization headers.

### Edge deployment

The pilot uses a separate Cloudflare Worker named `ajent-skills`, with custom
domain `skills.ajent.social`. It shares no Ajent database or credentials. The
Go implementation validates and exports the public snapshot; the small edge
adapter serves that snapshot. It has no outgoing fetches, secrets or bindings.
Cloudflare's existing free-plan limits apply; no plan upgrade is required.

```
go build -o /tmp/skillsd ./cmd/skillsd
/tmp/skillsd -origin https://skills.ajent.social -export > /tmp/skills-snapshot.json
SKILLS_SNAPSHOT=/tmp/skills-snapshot.json node --test deploy/skills/worker.test.mjs
node deploy/skills/bundle.mjs /tmp/skills-snapshot.json /tmp/skills-worker.mjs
```

Upload the generated module as `worker.mjs` with `deploy/skills/metadata.json`
using the Workers multipart upload API, then attach the custom domain. Keep
the previous deployed module available for rollback. Rebuild from the reviewed
manifest for every publication; never upload an arbitrary local corpus.
Observability and invocation logs are disabled, with query redaction configured
if logging is later enabled. Infrastructure may still process ordinary request
metadata. Do not send sensitive queries.

The zone configuration rule `ajent_skills_public_reads` disables only Browser
Integrity Check for GET/HEAD on this exact hostname. Without it, Python's
default HTTP client received error 1010. Other host rules remain in effect.
The service itself rejects writes. A global rate limit and larger index/storage
design remain prerequisites for a broad public launch; this deployment is a
small pilot under the free platform's quotas.

Set `SKILLS_GO_ORIGIN` to a running local Go service when running the Node
tests to exercise cross-runtime ranking parity. Without it that check is
explicitly skipped. Snapshot generation and the bundled adapter are both
required when changing the catalog. The lexical scanner is suitable for this
pilot; the 100,000-record validation ceiling is not a performance claim.

## Wire contract

- `GET /` and `GET /SKILL.md`: UTF-8 Markdown bootstrap, short cache lifetime.
- `GET /v1/skills/search?q=...&limit=5`: `ajent.skills.search/v1` envelope,
  `catalog_version`, and `results`. Default five, maximum ten results. `q` is
  at most 2,048 UTF-8 bytes. Unknown/duplicate query fields are rejected.
- Results contain `id`, `version`, `name`, `description`, `content_sha256`,
  `content_url`, `source_url`, `license`, `license_url`, `quality_status`, and
  `prerequisites`. Clients should tolerate additive response metadata.
- `GET /v1/skills/{id}/{version}`: exact Markdown instruction bytes, ETag,
  X-Content-SHA256, immutable caching. Version is lowercase content SHA-256.
  An unknown version returns 404; it never falls back to a different version.
- `GET /v1/licenses/{id}/{version}`: full source license notice.
- `GET /healthz`: status, catalog version, and count.
- Errors: `{"error":{"code":"invalid_query","message":"..."}}` with the
  corresponding HTTP status. Error messages never echo queries or headers.
- GET and HEAD only. No authentication is required or forwarded. Existing
  Ajent credentials belong to their exact issued origin, not this subdomain.

Search uses BM25 over names and short descriptions with stable tie ordering.
No-match returns `results: []`. This is lexical relevance, not a calibrated
quality score. The server caps response sizes through bounded record fields;
agents should still use result limits and inspect only chosen bodies.

## Catalog and review

The default manifest is `internal/skillcatalog/seed.json`. `-catalog FILE`
loads an explicit replacement manifest after full startup validation. It is
never read from a cloned project's configuration automatically. The operator
must retain historical entries needed by existing version references; removing
one causes 404, never silent replacement. Cache retained bytes with their hash.

Each record has identity, description, content, source URL, license and complete
license text, prerequisites, quality status, and `redistribution_reviewed: true`.
The current supported licenses are MIT and Apache-2.0. This flag is an operator
attestation, not automated proof of rights. Do not set it from repository-level
license metadata alone. Empty catalogs, duplicate bodies, oversized content,
unsupported licenses and unreviewed records fail startup. Catalog files are
limited to 64 MiB and each instruction body to 256 KiB.

The three seed skills were discovered in the skills.sh-derived Skillrouter
snapshot and retrieved afresh from Matt Pocock's public MIT repository at
revision `c55ee46073ed923f86ce59a5eb3b6d895095d1b7`. The selected files and their
ancestor directories have no overriding license notice. Original instruction
bytes and the full repository copyright/license are retained. `reviewed` means
source, notice, content and instruction-only prerequisites were inspected; no
comparative task-quality rating is asserted. Two explicit-invocation skills
remain explicit-invocation skills. The third requires the host to resolve its
subagent prerequisite under existing policy. Wrapper skills and packages with
required support files are deferred until package manifests are supported.

Personal files are not imported or uploaded by the service. A private corpus
needs authenticated visibility filtering and a separate publication review.
No endorsement, download popularity, or retrieval score grants execution rights.

## First-use acceptance

Install the bootstrap only, or ask an agent to read the service root. Request a
handoff summary. Observe search, inspect the matching metadata, retrieve the
pinned handoff skill, follow it within the task's permissions, and verify the
output. A second fetch must have identical bytes/hash. Exercise a no-match
query, unavailable service and missing prerequisite without invented success.

Local transport tests do not prove deployed DNS/TLS or a harness's automatic
skill trigger behavior. Record those observations separately before claiming
the hosted workflow is ready.

### Verification on 2026-09-24 UTC

- Focused Go race tests, vet and build passed for `internal/skillcatalog` and
  `cmd/skillsd`, under the shared build lease.
- Three Node test groups passed: discovery/content/license checks, malformed
  requests/no-match, and six-query ranking parity against the running Go service.
- Live trusted HTTPS checks passed using curl and Python's default HTTP client:
  root, search, pinned raw content, matching SHA-256, identical repeat fetch,
  source license and empty results. The Python 1010 failure was reproduced and
  fixed with the exact-host GET/HEAD configuration rule described above.
- One user-level `ajent-skills` bootstrap was installed and matched against live
  bytes. The existing Claude skills symlink exposes that same file. A manual
  handoff was produced in an OS temporary directory using the fetched procedure.
- No isolated fresh-harness trigger test, independent task-quality benchmark,
  CLI wrapper completion or large-corpus performance result is claimed.
