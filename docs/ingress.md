# Reaching Mustur from a phone

Milestone 2c's surface is built and bound to loopback. The last step —
making it reachable off the home network — is not a file in this repository and
cannot be. This is what it takes, what is true today, and who has to do it.

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

**302** is the answer you want. **200 or 502 means Access is no longer in
front**, and the service should be stopped until it is.

### 2. The hostname — done

`mustur.devofpie.com` → `http://127.0.0.1:7777`, on the existing tunnel. Port
`7777` is `mustur serve`'s default, and `serve` refuses any address that is not
loopback, so the tunnel is the only way in by construction.

### 3. Something listening — done

`deploy/mustur.service` is a systemd **user** unit: it runs as the account that
owns the store, needs no root, restarts on failure, and starts at boot on the
lingering this account already had.

```sh
make install-service            # build the binary and install the unit
systemctl --user enable --now mustur
```

Verified enabled and active, serving 91 records; it survives `systemctl restart`
and comes back from `kill -9` within seconds. Confined to two writable paths —
the store and the export tree — with `ProtectSystem=strict` and `ProtectHome`
read-only.

If Access is ever removed from the hostname, stop this first:

```sh
systemctl --user disable --now mustur
```

## What is still not demonstrable

One sentence, and it is the milestone's own: *a jot from a phone lands in
Mustur's findings-queue in seconds*. Only the owner can test it, because only
the owner can get through Access.

Everything under it is proven. A jot filed through the running service on
loopback took **20 ms** end to end, was routed by the guess to
`DevOfPie/Mustur`, and appeared in `records/findings.md` — the file the
`findings` role is mapped at — without anybody running `make export`. That is
`MUS-F-0022`, filed by the surface rather than by the command line, which is
what makes it the proof rather than a note about one.

What the phone adds to that path is Access and the network. Neither is measured
here, and the fifteen-second claim stays unclaimed until somebody holding a
phone off the home network files one and says what it took.
