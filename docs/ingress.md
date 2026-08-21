# Reaching Mustur from a phone

Milestone 2c's surface is built and bound to loopback. The last step —
making it reachable off the home network — is not a file in this repository and
cannot be. This is what it takes, what is true today, and who has to do it.

**Read on 2026-08-21: the hostname exists and is not protected.**
`https://mustur.devofpie.com/` is routed to `127.0.0.1:7777` and answers
unauthenticated requests directly — no Access login, no challenge headers, a
plain 502 from the origin path because nothing is listening yet. That 502 is
the only thing standing between the intake box and the open internet, and it
stops being true the moment the service starts.

## What is true, read on 2026-08-20

| | |
| --- | --- |
| `cloudflared` version | 2026.8.2 (built 2026-08-14) |
| How it runs | `linkctrl-tunnel.service`, `cloudflared tunnel run`, under `DynamicUser=yes` — the account is `linkctrl-tunnel` and exists only while the unit does. An earlier version of this row said "the `linkctrl` user"; there is no such account, and a review caught it |
| How it is configured | A `TUNNEL_TOKEN` in `/etc/cloudflared/env`. **Remotely managed** |
| Local ingress file | None. There is no `/etc/cloudflared/config.yml` on this machine |
| Public hostname | `mustur.devofpie.com` → `http://127.0.0.1:7777`, added by the owner 2026-08-21 |
| Access in front of it | **No.** An unauthenticated `GET /` returns the origin's 502 rather than a login redirect |
| Anything listening on 7777 | No. `deploy/mustur.service` exists and is deliberately not enabled |

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

Two of the three steps below are done. The one that is not is the one that has
to come first, and the order is not a preference.

### 1. An Access application — not yet done, and blocking

The intake surface reads the filer's identity from
`Cf-Access-Authenticated-User-Email`, a header Cloudflare Access sets at the
edge. `cloudflared` passes client headers through to the origin. With no Access
application in front, anyone who reaches the hostname can send that header
themselves and file a jot under any address they like — and can file jots at all.

In the Zero Trust dashboard (`one.dash.cloudflare.com`):

| Step | |
| --- | --- |
| 1 | **Access → Applications → Add an application → Self-hosted** |
| 2 | Name it `Mustur`. Session duration is a preference; 24 hours is reasonable for a phone |
| 3 | **Public hostname**: subdomain `mustur`, domain `devofpie.com`, path empty |
| 4 | **Add policy** — name `Owner`, action **Allow**, one include rule: *Emails* → `dev@killerofpie.com`. No other rule, and no `Everyone` |
| 5 | Save. Leave every other default alone: no bypass policy, no service-token policy, and nothing on the `Everyone` selector |

Then confirm it took, from anywhere:

```sh
curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' https://mustur.devofpie.com/
```

A **302 to a `cloudflareaccess.com` login** is the answer you want. A **502**
means the request still reached the origin and Access is not in front.

One thing worth setting while you are there: Access can strip inbound copies of
its own headers, but do not rely on that alone. The surface will trust that
header only because Access is the only thing that can set it, which is exactly
why the order matters.

### 2. The hostname — done

`mustur.devofpie.com` → `http://127.0.0.1:7777`, on the existing tunnel. Port
`7777` is `mustur serve`'s default, and `serve` refuses any address that is not
loopback, so the tunnel is the only way in by construction.

### 3. Something listening — ready, not started

`deploy/mustur.service` is a systemd **user** unit: it runs as the account that
owns the store, needs no root, restarts on failure, and starts at boot once
lingering is on.

```sh
make install-service            # installs it, does not enable it
loginctl enable-linger "$USER"  # so it survives a logout and starts at boot
systemctl --user enable --now mustur
```

**Do not run that last line before step 1 is confirmed.** Enabling the service
is what publishes the box; everything before it is inert.

## What is still not demonstrable

`mustur serve` was not a service, which is why the hostname 502s;
`deploy/mustur.service` closes that (`MUS-F-0014`) and is waiting on step 1.

Until Access exists and the service is enabled, the milestone's own wording — *a
jot from a phone lands in Mustur's findings-queue in seconds* — is demonstrable
on loopback and not from a phone. What has been measured is on loopback: ten
filings, median 0.5 ms, worst 0.9 ms; independently re-measured at median
0.35 ms, worst 0.55 ms. The page is 3,071 bytes empty and grows with the recency
list. The fifteen-second claim is about a network that is not connected yet, and
this file exists so that nobody records it as met before it is.
