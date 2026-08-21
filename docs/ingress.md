# Reaching Mustur from a phone

Milestone 2c's surface is built and bound to loopback. The last step —
making it reachable off the home network — is not a file in this repository and
cannot be. This is what it takes, what is true today, and who has to do it.

**Nothing here has been applied.** Everything under "What is true" was read off
this machine; everything under "What is needed" has not been done.

## What is true, read on 2026-08-20

| | |
| --- | --- |
| `cloudflared` version | 2026.8.2 (built 2026-08-14) |
| How it runs | `linkctrl-tunnel.service`, `cloudflared tunnel run`, under `DynamicUser=yes` — the account is `linkctrl-tunnel` and exists only while the unit does. An earlier version of this row said "the `linkctrl` user"; there is no such account, and a review caught it |
| How it is configured | A `TUNNEL_TOKEN` in `/etc/cloudflared/env`. **Remotely managed** |
| Local ingress file | None. There is no `/etc/cloudflared/config.yml` on this machine |

That last row is the one that decides the shape of everything below. A
token-managed tunnel takes its ingress rules from Cloudflare, not from disk, so
there is no local file to propose a diff against and no `make` target that could
verify one. Writing a local `config.yml` would not add a rule; depending on how
`cloudflared` resolves the two, it could replace every rule the tunnel already
serves. **Do not add one.**

The tunnel is LinkCtrl's. [Plan.md](../Plan.md#stack) already decided that
Mustur rides the existing tunnel rather than starting a second one, and cites
[cloudflare/cloudflared#59](https://github.com/cloudflare/cloudflared/issues/59)
for why. Sharing it means a Mustur outage and a LinkCtrl outage have a common
cause, which is the cost that decision accepted.

## What is needed

Three things, in this order. All three are the owner's: they are changes to a
Cloudflare account, and nothing in this repository holds a credential for one.

**1. A public hostname routed to the intake port.** In the dashboard, under the
tunnel's public hostnames, or through the API:

```
hostname   mustur.<the domain>
service    http://127.0.0.1:7777
```

`7777` is `mustur serve`'s default. It has to be running, which today means a
terminal on this machine — see the gap below.

**2. An Access application in front of that hostname**, before the route is
saved and not after. The intake surface trusts
`Cf-Access-Authenticated-User-Email` to say who filed a jot, and a hostname
published without Access in front of it means anything that can reach the port
can claim to be anyone. The free tier covers 50 users;
[Plan.md](../Plan.md#stack) puts identity at the edge for this reason.

**3. A policy allowing the owner's identity**, and nothing wider. Milestone 6 is
where a second person is added deliberately, with its own verdict.

## The gap this leaves, and it is real

`mustur serve` is not a service. It runs in a terminal, it does not start at
boot, and it does not restart if it dies — so a hostname pointed at port 7777
would 502 whenever nobody had started it by hand. A `systemd` unit is the
obvious answer and no milestone has asked for one; it is
[a finding](../records/findings.md) rather than something smuggled in here.

Until all of this exists, the milestone's own wording — *a jot from a phone
lands in Mustur's findings-queue in seconds* — is demonstrable on loopback and
not from a phone. What has been measured is on loopback: ten filings, median 0.5 ms, worst 0.9 ms;
independently re-measured at median 0.35 ms, worst 0.55 ms. The page is 3,071
bytes empty and grows with the recency list — 4,112 bytes after ten filings — so
the single figure an earlier version of this file gave was one reading of a
transient page, which a review said plainly. The fifteen-second claim is about a
network that is not connected yet, and this file exists so that nobody records
it as met before it is.
