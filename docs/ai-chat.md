# Ask Ajent: floating AI chat

The floating launcher appears on every HTML page, including sign-in and domain
verification. There is no standalone chat page: legacy GET `/ask` redirects
to `/?ask=1`, opening the floating conversation on the homepage. Existing Ask
links open it in place; without JavaScript they lead to `/guide`.
The implementation adapts sirelog/sire-chat's conversation bubbles, input bar,
retrieval/source pattern, and model-to-guide fallback into Ajent's existing
Go/templates/CSP and visual tokens. No Sire corpus or branding is included.
The original prototype's unused voice control and fake thread identifier are
not carried over. Follow-ups send up to three actual previous exchanges.

`POST /api/chat` accepts JSON `messages`, alternating user/assistant and ending
with a user question. Only the user's typed conversation and Ajent's published
guide are sent to OpenRouter. No live agent/directory queries, private network
content, project files, browser page content, or session credentials enter the
model prompt. The database stores only quota counters, not conversations or raw
IP addresses. Chat history is kept in page memory and discarded on reload or
New chat. Closing the panel preserves the conversation on that page.

The default model is `google/gemini-2.5-flash-lite`, configurable through
`ASK_MODEL`. Answers are capped at 600 output tokens, user messages at 2,000
characters, recent history at 12,000 characters, request bodies at 48 KiB and
upstream response bodies at 64 KiB. OpenRouter requests time out after 25
seconds and request provider routing with `data_collection: deny`. Visitors see
a disclosure that their typed conversation is sent to OpenRouter and its model
provider. Do not promise zero retention beyond these controls.

The model receives all current guide entries and returns an answer plus guide
IDs. Only recognized IDs become source links. The browser renders answers as
plain text, with links separately constructed from allowlisted guide paths;
model HTML and arbitrary links cannot execute. Citations establish which guide
entries were linked, not automatic verification that every generated claim is
correct. The chat tells users to check sources.

Default shared limits: 250 upstream requests per UTC day across the service,
20 per network address per UTC hour (IPv6 grouped by /64). Migration011 stores
atomic reservations across the rolling fleet, including failed provider calls.
`ASK_DAILY_LIMIT` can be set between 0 and 10000. These are request/token limits,
not a guaranteed dollar budget; model prices can change. Invalid requests and
missing-key guide fallbacks do not use paid quota. Quota failures make no model
request. Failed/invalid model responses fall back to matching published guide
text with an explicit mode label; they are never presented as successful AI.
No automatic retry increases upstream consumption.

Local development reads only `OPENROUTER_API_KEY`, `ASK_MODEL`, and
`ASK_DAILY_LIMIT` from `.env`; existing process environment takes precedence.
The dotenv file is ignored by Git, never executed, and unrelated credentials
are not imported. Production stores the OpenRouter key in AWS Secrets Manager.
Pulumi receives only `openrouterSecretArn`, grants the runtime role access to
that ARN, and fetches its value into the private runtime environment. No key is
embedded in HTML, JS, images, user data, or application source. Upstream errors
are deliberately not echoed into logs or responses.

The static guide remains at `/guide` for visitors without JavaScript. Legacy
POST `/ask` redirects there without copying the question into the URL.
Model answers do not post to the network or perform account actions.

The chat has no enclosing frame. Individual translucent glass bubbles float
above the host page; the conversation fades out at its top edge. The input stays
anchored below the scrollable thread. The original Sire design supplies the
glass blur, directional chip edges and gradient send ring, using Ajent colors.

References verified September 7, 2026:
- https://openrouter.ai/docs/api_reference/overview
- https://openrouter.ai/google/gemini-2.5-flash-lite

Validation so far: a real request using David's OpenRouter account returned AI
mode with a valid guide citation. Provider-contract tests cover actual history,
source-ID filtering, malformed/oversized upstream responses and sanitized errors.
Route tests cover cross-origin rejection, input bounds and explicit guide
fallback. PostgreSQL tests verify quota sharing between server instances and no
raw-IP storage. Chromium at 390/1024/1440/2880 checks followups, source links,
HTML injection resistance, Escape, reset, and question preservation after 429.
Infrastructure tests parse every embedded bootstrap program and assert the
secret-fetch path; no key is copied into a release artifact.

The container CI job retains its smoke-tested ARM64 image for one day, labeled
with the source revision. Releases can load that exact artifact and publish it
to ECR instead of recompiling on the shared development machine. Verify the
workflow, commit, successful checks, artifact name, and image revision label
before publishing; never substitute an artifact from an untrusted PR.
