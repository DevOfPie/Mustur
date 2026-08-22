# Reaching Mustur from a phone

Milestone 2c's surface is reachable off the home network. Making it so was never
a file in this repository and could not be, so this file is the record of what
it took, what is true now, and which part is still somebody's to do.

**Live and gated, 2026-08-21.** `https://mustur.devofpie.com/` is routed to
`127.0.0.1:7777`, Cloudflare Access sits in front of it, and the service that
answers it starts at boot. What remains unproven is the milestone's own
sentence — a jot filed *from a phone* — which only the owner can test, because
only the owner can get through Access.

## What is true, read on 2026-08-21

| | |
| --- | --- |
| `cloudflared` version | 2026.8.2 (built 2026-08-14) |
| How it runs | `linkctrl-tunnel.service`, `cloudflared tunnel run`, under `DynamicUser=yes` — the account is `linkctrl-tunnel` and exists only while the unit does. An earlier version of this row said "the `linkctrl` user"; there is no such account, and a review caught it |
| How it is configured | A `TUNNEL_TOKEN` in `/etc/cloudflared/env`. **Remotely managed** |
| Local ingress file | None. There is no `/etc/cloudflared/config.yml` on this machine |
| Public hostname | `mustur.devofpie.com` → `http://127.0.0.1:7777`, added by the owner 2026-08-21 |
| Access in front of it | **Yes**, added by the owner 2026-08-21 |
| Anything listening on 7777 | `mustur.service`, a systemd user unit, enabled and active |

The "how it is configured" row is the one that decides the shape of everything
below. A token-managed tunnel takes its ingress rules from Cloudflare, not from disk, so
there is no local file to propose a diff against and no `make` target that could
verify one. Writing a local `config.yml` would not add a rule; depending on how
`cloudflared` resolves the two, it could replace every rule the tunnel already
serves. **Do not add one.**

The tunnel is LinkCtrl's. [Plan.md](../Plan.md#stack) already decided that
Mustur rides the existing tunnel rather than starting a second one, and cites
[cloudflare/cloudflared#59](https://github.com/cloudflare/cloudflared/issues/59)
for why. Sharing it means a Mustur outage and a LinkCtrl outage have a common
cause, which is the cost that decision accepted.

## What was needed

All three steps below are done. They are kept in order because the order is not
a preference: the gate had to exist before anything listened, and on the next
host it will have to again.

### 1. An Access application — done

The intake surface reads the filer's identity from
`Cf-Access-Authenticated-User-Email`, a header Cloudflare Access sets at the
edge, and `cloudflared` passes client headers through. Access is what makes that
header trustworthy, which is why it had to exist before anything listened.

Confirmed against four requests, not one:

| Request | Answer |
| --- | --- |
| `GET /` | 302 to `killerofpie.cloudflareaccess.com`, `auth_status: NONE` |
| `GET /intake` | 302, same |
| `POST /intake` | 302, same — the write path is gated, not only the read |
| `GET /intake` carrying a forged `Cf-Access-Authenticated-User-Email` | 302, same. A header cannot buy a session |

To re-check it at any time:

```sh
curl -sS -o /dev/null -w '%{http_code}\n' https://mustur.devofpie.com/
```

**302** is the answer you want. **200 means Access is no longer in front**, and
the service should be stopped until it is. **502 means Access is still in front
and the origin is down** — Access answers before the origin is consulted, so
this check says nothing about whether anything is listening. It is a gate check,
not a health check, and nothing here watches the origin.

#### What these four requests do not establish

They show that an unauthenticated request is turned away. They say nothing about
**who the policy lets in**, and that is a separate requirement:

> **A policy allowing the owner's identity**, and nothing wider. Milestone 6 is
> where a second person is added deliberately, with its own verdict.

Cloudflare's free Zero Trust tier was recorded as covering 50 users when this
file first said so on 2026-08-20; nobody has re-checked it against Cloudflare's
pricing since, so treat it as an assertion. [Plan.md](../Plan.md#stack) puts
identity at the edge for this reason — so a policy accidentally scoped to a whole domain
would still answer 302 to every check above and still be wrong. The policy lives
in Cloudflare's dashboard, no credential on this machine can read it, and
nothing in this repository can verify it. **It is the owner's to confirm**, and
until they do it is an assertion rather than a measurement.

### 2. The hostname — done

`mustur.devofpie.com` → `http://127.0.0.1:7777`, on the existing tunnel. Port
`7777` is `mustur serve`'s default, and `serve` refuses any address that is not
loopback, so the tunnel is the only way in by construction.

### 3. Something listening — done

`deploy/mustur.service` is a systemd **user** unit: it runs as the account that
owns the store, needs no root, restarts whenever it exits, and starts at boot on
the lingering this account already had.

```sh
make install-service            # build the binary and install the unit
systemctl --user enable --now mustur
```

Verified enabled and active. Confined to two writable paths — the store and the
export tree — with `ProtectSystem=strict` and `ProtectHome` read-only.

It restarts from `kill -9` (`status=9/KILL` 03:08:32, `Started` 03:08:37) and
from `kill -TERM` (`MainPID` 933302 → 934515, `NRestarts=1`, `/healthz` 200
after). The `TERM` case is the one worth naming: the unit shipped `Restart=on-
failure`, systemd counts a `TERM` as a clean exit, and a review's stray `pkill`
matched this unit's `ExecStart` and left the production service dead with no
restart scheduled and nothing watching. `Restart=always` is the fix. The check
above could not have caught it — Access answers 302 to a dead origin.

If Access is ever removed from the hostname, stop this first:

```sh
systemctl --user disable --now mustur
```

## The last sentence, demonstrated

*A jot from a phone lands in Mustur's findings-queue in seconds, carries a
routing hint where one is obvious, and defaults to the idea inbox where it is
not.* Only the owner could test it, because only the owner gets through Access,
and on **2026-08-22** they did — three jots from a phone, all three routed
correctly:

| Jot | Text | Went to | Because |
| --- | --- | --- | --- |
| `MUS-F-0023` | "Test for Mustur 2c" | `DevOfPie/Mustur` | the jot names Mustur |
| `MUS-F-0024` | the intake screen's file button has no hover or press state | `DevOfPie/Mustur` | the jot names Mustur |
| `MUS-F-0025` | "Test" | Idea inbox (`MUS-P-0002`) | no destination is obvious |

The third is the one that mattered, because it is the clause's default case and
the only one a routing guess cannot get right by accident. It also read as
mis-routed at first glance and was not — and the reason it read that way has
since been fixed. Every record in the store then carried the `MUS-` prefix,
which was the store's rather than the destination's; the owner's point was that
this makes a store you have to open rather than one you can scan. A routing
record now names its own prefix and intake files under it, so the next jot with
no obvious home is `IDW-F-0001` and not a Mustur record at all
([MUS-D-0093](../records/decisions.md#mus-d-0093)).

`MUS-F-0025` keeps its identifier. It is the last jot filed under the old
scheme, and renaming it would put an exception into the rule that makes every
citation in this tree safe.

`MUS-F-0024` is a real defect the owner noticed while proving the milestone,
which is the surface working as intended in a second sense.

Everything under it was already proven. A jot filed through the running service on
loopback was routed by the guess to `DevOfPie/Mustur` and appeared in
`records/findings.md` — the file the `findings` role is mapped at — without
anybody running `make export`. That is `MUS-F-0022`, filed by the surface rather
than by the command line, which is what makes it the proof rather than a note
about one.

**What that leg costs, measured.** Ten filings by `curl` against a
`--export`-enabled server on loopback, `%{time_total}` per request, sorted:
median **1.71 ms**, worst **2.10 ms**, best **1.43 ms**. Ten filings against the
same binary with export off measured slower on first use — median 7.05 ms — on a
cold page cache, so treat the fast figure as the warm steady state rather than
the cost of a first filing after boot. An earlier version of this file gave
**20 ms** for a single filing with no stated instrument; one reading of a
transient page is what a previous review already objected to, and this replaces
it. The older loopback figures it also replaces were median 0.5 ms, worst 0.9 ms
over ten filings, independently re-measured at median 0.35 ms, worst 0.55 ms,
against a store roughly a third smaller than today's.

The page grows with the recency list, which is why a single timing of it means
little. The size after ten filings is recorded twice in this repository and the
two disagree — 4,112 bytes in `decisions.md`, 4,212 in the record exported from
the store — and neither is reproducible now, because the list only holds the
last hour and the page has since gained a banner. Both are left standing rather
than one being quietly picked. What is measurable today: the intake page served
by the running service, with an empty recency list, is **2,245 bytes**, read
2026-08-21 with `curl … | wc -c`.

What the phone adds to that path is Access and the network. Neither is measured
here, and the fifteen-second claim stays unclaimed until somebody holding a
phone off the home network files one and says what it took.
