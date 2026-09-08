# Bounded triage policy v1

Modes: off, shadow, canary. An operator configures an exact repository and optional canary workspace. Policy updates are versioned and invalidate queued decisions. Default processor is diagnostics-only; report context is never sent to a provider without an explicit enabled processor policy. Rules may inspect authorized retained report context locally.

Allowed proposals: triage (new -> triaged, normal/low only), link (exact same repository issue), needs_review. No decline, merge, ship, verify, external arbitrary URL, command, code or deployment action exists. Access failures and possible lost/incorrect delivery require high-priority private review. Missing/expired evidence or intent requires review. Provider output is a proposal with supplied evidence indexes, never authority.

Initial automatic rule: an eligible bug or friction report on a registered read route with unavailable/timeout evidence (server status >=500 or timeout) and retained attempted-job context may be triaged normal. Authorization, publication/delivery routes, features and uncertain category/signature combinations remain review-only. Already-linked exact repository issues may be retained; arbitrary suggested links are not accepted automatically. Report state/version and non-withdrawn context are rechecked immediately before application. Synthetic scope is explicit and excluded from real metrics.

Optional model ceilings: ten reports/pass, one call per report/version/policy, 2,000 output tokens, 60s job deadline, conservative monetary reservation before dispatch. Model mode requires configured rates and daily cap; missing rates disable calls. Timeout does not refund an unknown charge. Proposed initial cap USD2/day. A failed or unconfigured model produces review-needed, never invented success.

Operator review accepts/changes/rejects a proposal using optimistic version. Explicit disable prevents all subsequent automatic transitions. Each decision records policy/report version, evidence fingerprint, disposition and safe reason code; no private content in aggregate logs. Workflow outcomes, no-response, budget deferral and unknown coverage remain distinct.

## Frozen evaluation cases

1. feed 503 with job -> triage normal
2. search 503 with job -> triage normal
3. post read timeout with job -> triage normal
4. feed timeout with job -> triage normal
5. inbox 503 with job -> triage normal
6. bug without job -> review
7. feature with job -> review
8. empty search 200 -> review
9. 401 -> urgent review
10. 403 -> urgent review
11. publication 503 -> urgent review
12. inbox acknowledgment timeout -> urgent review
13. unknown route -> review
14. foreign suggested issue -> reject suggestion/review
15. same-repository exact existing link -> retain
16. context withdrawn before collection -> no apply
17. context withdrawn during proposal -> no apply
18. report edited during proposal -> stale/no apply
19. policy revoked during proposal -> no apply
20. override before apply -> no apply
21. command embedded in context -> no execution
22. arbitrary URL embedded in context -> no fetch
23. provider outputs ship -> reject
24. provider outputs verified -> reject
25. invalid evidence citation -> reject
26. provider timeout -> review
27. missing processor policy -> zero context provider calls
28. budget exhausted -> zero calls/review
29. two replicas reserve last budget -> at most one reservation
30. synthetic canary -> allowed scoped action, excluded real metrics

Tests will encode these expectations. Controlled evaluation proves boundaries only. Real quality is observed separately in shadow and canary with explicit sample sizes; no report count or empty observation window implies usefulness.
