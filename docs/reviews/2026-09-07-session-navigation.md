# Session-aware public navigation

Signed-in visitors saw Sign in because the shared header was static and the public
front door intentionally removes cookies from upstream requests. The application
session cookie is host-only on dogfood.ajent.social.

The shared header now loads session-nav.js, which requests GET /session-status
directly from the application with credentials. The response contains only an
authenticated boolean, is no-store, and permits credentialed CORS only from
https://ajent.social when the application origin is https://dogfood.ajent.social.
The public Worker permits that origin in connect-src while continuing to strip
cookies and Authorization from public-page proxy requests.

Authenticated visitors see Dashboard linking to /app and no Join button. Anonymous
visitors see Sign in and Join ajent. Focus, visibility and pageshow events refresh
the state after login, logout and history restoration. Failed status requests
leave navigation usable. No session issuance, credential scope or publication
behavior changes.

## Validation

Source: 55fe53b645e846ec537a6ac320ee25df7300fd41.
CI: https://github.com/ajent-social/ajent-social/actions/runs/34125081159.
All three jobs passed. The Go race suite executed 102 tests, zero failures and
zero skips. The existing authenticated web test now checks anonymous, signed-in,
logged-out, trusted-origin and rejected-origin status requests, including the
exact boolean-only response and no-store headers. Ten Worker tests passed.

Twenty-four browser assertions at widths 390, 1024, 1440 and 2560 checked sign-in,
Dashboard, hidden Join, logout restoration, keyboard focus, overflow and service
failure behavior. These preliminary browser assertions used live page markup
with mocked session-state responses; production validation is recorded below.

## Release

Application image: sha256:a0b8977bcbf51f66c79ce8341c3bbcb2f6aba22d92b152c14175a55c62d5aaa1.
ECR scan completed with no reported findings. The Pulumi preview updates only
the launch template and autoscaling group; 41 resources remain unchanged.
Worker deployment: 3d1bd22282fe450a9103b2018d06573b.
Worker ETag: 1124e0f286d28816a0757f1595d2d8ea921b9f7190577a6287caa06866c70afa.
AWS instance refresh: 56c1b3e2-c7b8-4685-a9d5-c29fea89f8bb.

The refresh completed Successful at 100%, with two healthy instances on launch
template version 12. A real browser login using the existing installation account
then showed Dashboard on six public pages (home, docs, about, join, participation
and domains). Clicking Dashboard opened the authenticated workspace. Signing out
in a second tab restored Sign in and Join when returning to the public tab.
The live endpoint returned true while signed in, false after logout, no-store
headers, and HTTP 403 without a CORS grant for an untrusted origin. No page errors
were recorded. Mobile and desktop screenshots are stored alongside this receipt.

The first browser-run attempts used an incorrect test selector requiring an
explicit submit type on a button whose HTML default is submit. Correcting that
selector allowed the unchanged application to pass the full live flow.
