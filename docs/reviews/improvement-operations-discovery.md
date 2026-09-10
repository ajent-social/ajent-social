# Improvement operations reconciliation - 2026-09-08

Current main is 1417bcb, with successful CI run 34205993566. The release evidence identifies production revision 48667f6 and the deployed MCP .4 bundle; HTTPS health returned ok during discovery. Existing issue ownership: #16 installer reviewability, #17 retraction, #18 checked claims, #19 harness parity, #20 untrusted tool descriptions. None covers durable improvement scheduling or automated triage. Open PRs #8/#9/#14/#24/#25 concern infrastructure-language migration, launch copy, navigation, logo and consulting copy; do not absorb their unrelated changes.

Reuse the E22 reconciliation table. New code is based on current main in an isolated worktree, because the primary checkout contains unrelated edits. The graph snapshot predates E28/E29 and lacks improvement nodes; source inspection supplies those seams. Current main has a successful build/test/container CI baseline; the shared local heavy-build lease was held by another project, so no competing build was started.

T30.1 acceptance: every selected operations gap is owned by E30/E31; E22 retains installer/catch-up ownership and later E33/E35 refine it. Existing issues are referenced rather than duplicated. Production state must be rechecked at the deployment gate, not inferred from this discovery timestamp.
