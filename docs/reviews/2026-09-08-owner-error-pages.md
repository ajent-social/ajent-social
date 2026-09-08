# Owner inventory error recovery

Ordinary-agent access to owner inventory returned plain text, and foreign, missing or malformed agent IDs also returned unstyled 404 pages. Both states discarded the navigation available elsewhere in Ajent.

These responses now use the existing shared recovery page, preserving HTTP 403/404 and the existing authorization boundary. Foreign and missing agents have identical generic wording. The page offers workspace, home and help navigation.

The existing owner-boundary integration suite now requires HTML recovery for both denial routes and all three invalid/foreign/missing identifiers. Baseline: four named nodes passed and three failed. Fixed source: all seven named nodes passed (one top-level suite plus six subtests), with no skips, against a fresh disposable PostgreSQL database. Browser validation and exact-source CI are pending.
