// Generated from the reviewed public catalog. Run import-catalog.mjs; do not edit.
export const snapshot = {
  "source": {
    "repository": "https://github.com/ajent-social/capabilities",
    "revision": "9ea4343d64500a6894384ea598745c036d21f5c7",
    "date": "2026-09-22",
    "sha256": "c46334cd3675a2753500f937270a8b1a4ed5949fc36263528b773923f6753faf"
  },
  "capabilities": [
    {
      "id": "billing.customer-checkout",
      "domain": "billing",
      "name": "Customer linkage and checkout",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Associate local customers and recover checkout attempts safely.",
      "use_when": [
        "A product integrates hosted subscription checkout."
      ],
      "non_goals": [
        "Price engine",
        "Payment processing"
      ],
      "proposed_guarantees": [
        "Attempt identity survives retries",
        "Local customer association is explicit"
      ],
      "failure_modes": [
        "Response lost after provider success",
        "Concurrent duplicate customer creation"
      ],
      "open_questions": [
        "Define durable recovery and provider idempotency-window behavior."
      ],
      "alternatives": [
        {
          "name": "Stripe Go SDK",
          "url": "https://github.com/stripe/stripe-go",
          "assessment": "Delegate provider calls and signatures; investigate local orchestration."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "billing.subscription-reconciliation",
      "domain": "billing",
      "name": "Subscription reconciliation",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Maintain recoverable local subscription projections.",
      "use_when": [
        "Local access decisions depend on provider subscription state."
      ],
      "non_goals": [
        "Universal entitlement policy",
        "Multi-provider framework"
      ],
      "proposed_guarantees": [
        "Duplicate event application is harmless",
        "Recovery can converge to provider state"
      ],
      "failure_modes": [
        "Out-of-order events",
        "Concurrent reconciliations"
      ],
      "open_questions": [
        "Resolve ordering and multi-instance persistence semantics."
      ],
      "alternatives": [
        {
          "name": "Stripe webhooks",
          "url": "https://docs.stripe.com/webhooks",
          "assessment": "Use provider verification; local state/recovery still needs a contract."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "delivery.container-artifact",
      "domain": "delivery",
      "name": "Verified container artifact",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Produce a scanned and signed artifact tied to source.",
      "use_when": [
        "A product publishes deployable container images."
      ],
      "non_goals": [
        "Universal builder or deployer"
      ],
      "proposed_guarantees": [
        "Verified digest returned only after required checks"
      ],
      "failure_modes": [
        "Published image mistaken for approved image",
        "Mutable tag substitution"
      ],
      "open_questions": [
        "Define scan failure and provenance verification behavior."
      ],
      "alternatives": [
        {
          "name": "Sigstore Cosign",
          "url": "https://docs.sigstore.dev/cosign/",
          "assessment": "Use established signing tooling; specify promotion gates."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "delivery.go-release",
      "domain": "delivery",
      "name": "Go release verification",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Verify source revision, version and expected release assets.",
      "use_when": [
        "A Go application publishes binary releases."
      ],
      "non_goals": [
        "Universal version scheme"
      ],
      "proposed_guarantees": [
        "Required assets checked before publication",
        "Version matches intended release"
      ],
      "failure_modes": [
        "Wrong version stamp",
        "Missing architecture asset"
      ],
      "open_questions": [
        "Set source/tag binding and product gate preservation."
      ],
      "alternatives": [
        {
          "name": "GoReleaser",
          "url": "https://goreleaser.com/",
          "assessment": "Prefer existing release tooling; share justified validation around it."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "delivery.go-validation",
      "domain": "delivery",
      "name": "Go validation",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Run bounded credential-free Go checks with predictable outcomes.",
      "use_when": [
        "A Go module needs CI validation."
      ],
      "non_goals": [
        "Universal CI orchestration"
      ],
      "proposed_guarantees": [
        "Explicit module/toolchain",
        "Failed checks propagate"
      ],
      "failure_modes": [
        "Required check skipped",
        "Caller events lost"
      ],
      "open_questions": [
        "Verify in two real modules without weakening caller gates."
      ],
      "alternatives": [
        {
          "name": "GitHub reusable workflows",
          "url": "https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows",
          "assessment": "Use workflow_call for complete jobs."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "delivery.infrastructure-preview",
      "domain": "delivery",
      "name": "Infrastructure preview",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Run infrastructure previews with explicit authority and results.",
      "use_when": [
        "Changes need provider-aware infrastructure review."
      ],
      "non_goals": [
        "Automatic production apply"
      ],
      "proposed_guarantees": [
        "Narrow preview identity",
        "Bounded outcome reporting"
      ],
      "failure_modes": [
        "Untrusted code accesses secrets",
        "Preview mistaken for enforcement"
      ],
      "open_questions": [
        "Set trusted-event rules and live permission tests."
      ],
      "alternatives": [
        {
          "name": "Pulumi GitHub Actions",
          "url": "https://www.pulumi.com/docs/iac/using-pulumi/continuous-delivery/github-actions/",
          "assessment": "Reuse native tooling with constrained caller trust."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "identity.password-hashing",
      "domain": "identity",
      "name": "Password hashing",
      "status": "REJECTED",
      "disposition": "REFERENCE_EXISTING",
      "summary": "Do not create an AMSL password hashing primitive.",
      "use_when": [
        "Password authentication is required."
      ],
      "non_goals": [
        "New cryptographic algorithm"
      ],
      "proposed_guarantees": [
        "Delegate cryptographic primitives to maintained libraries"
      ],
      "failure_modes": [
        "Unsafe parameters or secret leakage"
      ],
      "open_questions": [
        "Reconsider only for a demonstrated integration gap, not new crypto."
      ],
      "alternatives": [
        {
          "name": "Go x/crypto",
          "url": "https://pkg.go.dev/golang.org/x/crypto",
          "assessment": "Use a maintained password hashing implementation with reviewed parameters."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "identity.service-credentials",
      "domain": "identity",
      "name": "Scoped service credentials",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Issue, verify, expire and revoke secret service credentials.",
      "use_when": [
        "An application admits authenticated machine requests."
      ],
      "non_goals": [
        "General authorization engine",
        "Browser-publishable key model"
      ],
      "proposed_guarantees": [
        "Stable owner/resource binding",
        "Hashed secret verification",
        "Explicit expiry and revocation"
      ],
      "failure_modes": [
        "Cross-owner access",
        "Revoked credential accepted",
        "Secret disclosed in logs"
      ],
      "open_questions": [
        "Set storage atomicity, account-status and scope-checking contracts."
      ],
      "alternatives": [
        {
          "name": "Go crypto/rand",
          "url": "https://pkg.go.dev/crypto/rand",
          "assessment": "Use standard cryptography; AMSL may own the lifecycle integration."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "identity.sessions",
      "domain": "identity",
      "name": "Browser session lifecycle",
      "status": "DISCOVERED",
      "disposition": "REFERENCE_EXISTING",
      "summary": "Evaluate existing opaque-session implementations before extracting integration.",
      "use_when": [
        "An application needs revocable browser sessions."
      ],
      "non_goals": [
        "Universal token architecture"
      ],
      "proposed_guarantees": [
        "Proposed integration preserves expiration and revocation"
      ],
      "failure_modes": [
        "Stale session after account disable"
      ],
      "open_questions": [
        "Determine residual integration not already provided."
      ],
      "alternatives": [
        {
          "name": "SCS",
          "url": "https://github.com/alexedwards/scs",
          "assessment": "Evaluate session management before building another engine."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "infrastructure.deployment-identity",
      "domain": "infrastructure",
      "name": "Deployment identity",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Bind deployment authority to intended workload and resource scope.",
      "use_when": [
        "CI needs cloud access without static cloud keys."
      ],
      "non_goals": [
        "Universal cloud abstraction"
      ],
      "proposed_guarantees": [
        "Explicit trust subject and audience",
        "Explicit resource scope"
      ],
      "failure_modes": [
        "Overbroad trust",
        "Wrong environment assumes identity"
      ],
      "open_questions": [
        "Select one provider and publishable disposable verification environment."
      ],
      "alternatives": [
        {
          "name": "GitHub OIDC",
          "url": "https://docs.github.com/en/actions/concepts/security/openid-connect",
          "assessment": "Use provider federation; components encode narrow policy."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "infrastructure.private-database",
      "domain": "infrastructure",
      "name": "Private database profile",
      "status": "CANDIDATE",
      "disposition": "EXTRACT",
      "summary": "Specify protected database connectivity and lifecycle policy.",
      "use_when": [
        "An application needs a managed private database."
      ],
      "non_goals": [
        "Universal availability or retention defaults"
      ],
      "proposed_guarantees": [
        "Private reachability by default",
        "Explicit backup and deletion policy"
      ],
      "failure_modes": [
        "Public exposure",
        "Unverified restore procedure"
      ],
      "open_questions": [
        "Define one provider profile and cost/recovery choices."
      ],
      "alternatives": [
        {
          "name": "Pulumi",
          "url": "https://www.pulumi.com/docs/iac/",
          "assessment": "Use provider resources and policy checks; verify live behavior separately."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    },
    {
      "id": "protocol.mcp-server",
      "domain": "protocol",
      "name": "MCP protocol server",
      "status": "REJECTED",
      "disposition": "REFERENCE_EXISTING",
      "summary": "Use the official SDK instead of a new AMSL protocol implementation.",
      "use_when": [
        "A product exposes MCP tools."
      ],
      "non_goals": [
        "New MCP stack",
        "Product authorization policy"
      ],
      "proposed_guarantees": [
        "Delegate protocol behavior to an established SDK"
      ],
      "failure_modes": [
        "Tool accesses another tenant resource"
      ],
      "open_questions": [
        "Reconsider only a separately demonstrated integration gap."
      ],
      "alternatives": [
        {
          "name": "Official MCP Go SDK",
          "url": "https://github.com/modelcontextprotocol/go-sdk",
          "assessment": "Reuse the SDK; product registers tools and enforces resource authorization."
        }
      ],
      "implementations": [],
      "verification": [],
      "publicEvidence": [],
      "restrictedEvidence": true
    }
  ]
};
