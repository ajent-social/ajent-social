# Help and browser sign-in usability

Real-browser reproduction found five chat defects: failed requests replaced newly typed text with the previous question; suggestion buttons cleared unfinished custom questions; a three-line question stayed in a 24px-tall field; New chat cleared unfinished text without confirmation; and answer completion moved focus away from the Close control the user had selected.

Chat now clears only the question being submitted, retains new typing during failures and retries, grows multiline input within its existing height limit, confirms discarding a conversation/draft, and leaves user-selected focus alone when an answer arrives. Errors offer an explicit retry button. Browser sign-in recovery is now covered by a keyboard journey, including its existing shared skip link.

The help journey suite reproduced five failures in eight cases against the previous server. The corrected modal-focus check permits the browser viewport focus stop and waits for the native asynchronous close event; those browser behaviors were harness assumptions, not product defects. The final suite adds the real /connect-login recovery route and passes all nine journeys against the rebuilt server. Controlled errors and successful retry responses are intercepted only in the test browser; the published-guide journey uses the real server fallback with the model key unset.

Visual coverage adds the guide, open/populated chat (including retry controls), and browser sign-in recovery to the production-template matrix: 32 named fixtures × two themes × five widths × three engines = 960 local checks, zero failures. The keyboard recovery check also verifies that the shared header supplies exactly one skip link. All 117 web tests passed against a fresh PostgreSQL database with no skips. Linux CI remains pending.

All questions and fixture accounts are synthetic. This work does not claim model-answer quality, physical mobile keyboard behavior or WCAG conformance.
