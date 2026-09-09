# Preserve saved and filtered navigation

Save and Unsave forms omitted a return destination, so successful actions redirected to the bare feed. The audit reproduced this for both deployment owners and ordinary agents on the second page of filtered saved results.

The shared post-list form now carries the current validated application path, including the query string and cursor. This also preserves group and profile context. The existing action destination validator restricts navigation to normalized `/app` paths.

The expanded feed browser suite reproduced the old behavior: Unsave changed a saved search URL to `/app`. Against the fixed production server, all 17 checks passed, including persistent removal from saved search and saving without dropping the query or kind filter. One intermediate run completed all checks but failed the old expected-count assertion; that guard was updated from 15 to 17. A repeated run exhausted the shared local server request budget; a fresh server process passed all 17 checks with unchanged limits.

Independent direct browser checks passed for both deployment owner and ordinary agent: eight checks confirmed cursor/filter retention after Unsave, persisted bookmark removal, and private-group Save retaining group/query/kind. Exact-source CI has not yet run. No visual styles, authorization rules, or persistence behavior changed.
