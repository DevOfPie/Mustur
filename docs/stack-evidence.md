# Stack evidence

Research collected 2026-08-19 (whippy, overnight session) for the three
undecided rows in [Plan.md](../Plan.md#stack): language, storage, and how the
adapter supervises sessions. **This file decides nothing** — it carries the
evidence the decisions will be taken against, as prompts. Numbers were measured
on the whippy VM on 2026-08-19 or carry their source; anything unverified says
so.

VM baseline that day: go1.26.5, node v24.19.0, Python 3.14.4, Docker 29.7.2,
`claude` 2.1.235, `sqlc` present in `/usr/local/bin`.

## Language candidates

| | Go | Node/TS | Python |
| --- | --- | --- | --- |
| PTY supervision | `creack/pty` (2,075★; last tag v1.1.24 2024-10-31, master active) — used by gotty, agentapi, catnip | `microsoft/node-pty` v1.1.0 (2025-12-22, active) — used by VS Code, vibetunnel (as a fork), Crystal, claudecodeui | stdlib `pty`; the layer above is stalest: `ptyprocess` 0.7.0 (2020), `terminado` 0.18.1 (2024) |
| Server-rendered HTML | stdlib `html/template`; `a-h/templ` 10.5k★ active; htmx pairs cleanly | no stdlib templating — weakest SSR baseline of the three | Jinja2/Starlette/FastAPI, all current |
| MCP server over HTTP | official `modelcontextprotocol/go-sdk` v1.7.0 (2026-07-28, spec 2026-07-28); `NewStreamableHTTPHandler` is a plain `http.Handler` — mounts beside HTML routes in one mux | official SDK mid v1→v2 package split (2.0.0 landed 2026-07-27; monolith `latest` still 1.30.0) — every tutorial in circulation is v1-shaped | official `python-sdk` v2.0.0 (2026-07-28), most-starred MCP SDK (24k★); mounting has two documented traps (session-manager lifespan, `Mount("/")` ordering) |
| Claude Agent SDK | **none official** — headless `--output-format stream-json` or PTY, hand-rolled | official `@anthropic-ai/claude-agent-sdk` 0.3.235 (2026-08-18) | official `claude-agent-sdk` 0.2.140 (2026-08-18) |
| Deploy weight (measured) | **8.3 MB** static binary (`CGO_ENABLED=0`, `-s -w`) | node binary 126 MB, runtime tree 224 MB, plus a native addon | 7.5 MB binary + 56 MB stdlib + venv; no single binary without PyInstaller |
| Honest liability | no Agent SDK: the stream-json wire is yours to speak (vibe-kanban and Sculptor did exactly this, in Rust and Python) | native-addon ABI churn tracking VS Code's Node, not yours; SDK migration noise | least-maintained PTY layer of any candidate; N concurrent PTYs under asyncio is hand-rolled |

All three official MCP SDKs arm DNS-rebinding/Host-allowlist protection by
default; behind a tunnel hostname this rejects every request (`421 Misdirected
Request` in the Python docs' words) until the allowlist is configured. Whatever
the language, that is a deploy-day footgun to plan for.

## What the category did, 2025–2026

A ~30-project survey of "web UI over terminal agent sessions" (full report in
the session evidence; stars and dates read from the GitHub API 2026-08-19):

- **Owning sessions is the norm; attaching is nearly extinct.** Genuine attach
  survives in Anthropic's Remote Control, vibetunnel's tmux adoption, and
  `opencode attach`. Everything else spawns the agent itself — the shape this
  plan already chose.
- **Three supervision architectures.** tmux-backed (claude-squad, Codeman,
  agent-of-empires) gets crash-survival and scrollback free, and a human can
  still `tmux attach` to a session Mustur started. Direct PTY (vibetunnel,
  claudecodeui, catnip) gets fidelity and hand-rolls a replay buffer
  (claudecodeui: 5,000-chunk rolling buffer, PTY survives disconnect). Headless
  `stream-json` over stdio (vibe-kanban, Sculptor, Crystal) skips terminals and
  gets structured messages — and needs no Agent SDK.
- **Nobody in the survey server-renders HTML** — every client is a SPA, mostly
  WebSocket, some SSE. Mustur's SSR principle has no prior art in this
  category; that is a fact to hold, not an argument either way.
- **Ten exits in eight months**, including the category leader: bloop/Vibe
  Kanban (27.8k★) shut its hosted layer 2026-04-10 and the repo froze;
  Terragon died entirely 2026-02-09. The stated killer, from a casualty's
  archive note: Anthropic's own Remote Control and Claude Code Web covered
  their use cases. The one articulated counter-moat (claudecodeui): "all your
  sessions, not just one". Mustur's moat is the same shape — the routing and
  records, not the terminal.
- **ACP is emerging as the neutral interface.** OpenHands, agent-of-empires
  and agentapi's experimental path all drive Claude through
  `@agentclientprotocol/claude-agent-acp` instead of TUI-scraping or per-CLI
  flags. The adapter's vendor-neutrality boundary (Plan.md stack table) may
  eventually be ACP rather than "shell out to the configured CLI" — untested,
  noted for the second-CLI milestone.
- **Metering risk, live:** Anthropic announced 2026-05-13 that Agent SDK /
  `claude -p` usage would leave plan limits for separate credits, then paused
  the change indefinitely on 2026-06-15 (support article 15036540). If it
  un-pauses, every headless/SDK supervisor pays per-token while interactive
  sessions stay on the plan. This cuts across the supervision choice and is
  queued for a Plan.md limitation row.

## Cloudflare facts for milestones 4–6

- **Second public hostname: one dashboard route, no second tunnel, no
  restart.** Networking → Tunnels → tunnel → Routes → Add route (Published
  application). Remote config is pushed to the running connector over the
  tunnel control RPC. Caveat, stated: no single docs sentence says "no restart
  needed" — it is the documented push mechanism plus the absence of any restart
  step. cloudflared#59's own closure points at the many-services-one-cloudflared
  ingress model.
- **Access path rules:** no port numbers, no query strings, no `#` anchors in
  application paths (docs state all three). Several apps share one hostname by
  path, most-specific wins; a path no app covers is **unprotected** — so a
  catch-all app on the hostname is the floor, with narrower apps layered on.
- **The second person:** One-time PIN is no longer a default login method
  (changelog 2026-06-18) — add it explicitly as an identity provider, then
  Allow + Include: their email + Require: One-time PIN. **Access cannot express
  "read-only"** — actions are Allow/Block/Bypass/Service Auth, binary per app.
  Read-only is Mustur's job, or a separate hostname/path as its own app. The
  free tier's seat number is no longer published in primary docs (the 50-seat
  figure survives only in marketing copy — unverified).
- **Agents through the same door:** a service token (`CF-Access-Client-Id` /
  `CF-Access-Client-Secret` headers) against a Service Auth policy protects an
  MCP-over-HTTP endpoint for non-browser clients; documented for agents
  2026-06-26. Service Auth carries no user identity — scope such tokens
  narrowly, in their own policy. Never Bypass.
