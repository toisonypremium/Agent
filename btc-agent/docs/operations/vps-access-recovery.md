# VPS access and reboot recovery runbook

Last verified: 2026-07-18
Host: `lelinh`

## Primary access

The authoritative working route is direct SSH:

```text
Public IPv4: 14.235.13.71
Port: 2222
User: admin
Key: ~/.ssh/id_ed25519_9router_sync
```

Termux command:

```bash
ssh -p 2222 \
  -o IdentitiesOnly=yes \
  -i /data/data/com.termux/files/home/.ssh/id_ed25519_9router_sync \
  admin@14.235.13.71
```

Local alias:

```bash
ssh vps-ssh
```

`vps-ssh` currently resolves to the direct IP and port, not Cloudflare Access.

## Access routes

| Alias/route | Purpose | Current state |
|---|---|---|
| `vps-ssh` | Primary direct SSH | Verified |
| `vps-direct` | Explicit direct fallback | Verified |
| `vps-cloudflare` | Cloudflare Access fallback | Not operational: WebSocket handshake blocked |
| `100.93.4.11:22` | Tailscale fallback | VPS address exists; local Termux route was unavailable |

Do not point Cloudflare SSH at `vps.linhbot.xyz`. That hostname currently serves
an unrelated HTTP service and returns a WebSocket bad handshake to
`cloudflared access ssh`.

A dedicated CNAME `ssh-vps.linhbot.xyz` was routed to the named SSH tunnel, but
Cloudflare edge policy still returns HTTP Basic Auth before WebSocket upgrade.
It is not the primary route until its Access application/policy is corrected.

## Network topology

```text
VPS LAN IPv4: 192.168.1.110
Public IPv4:  14.235.13.71
Direct NAT/forwarded SSH port: 2222
Tailscale IPv4: 100.93.4.11
SSH origin for Cloudflare tunnel: 127.0.0.1:22
```

The public IPv4 may be dynamic. A reboot did not change it on 2026-07-18, but
that does not prove it is reserved/static.

## Post-reboot verification

Run through direct SSH:

```bash
hostname
uptime -s
systemctl --failed --no-pager
systemctl status ssh --no-pager
systemctl is-active tailscaled
systemctl --user is-active btc-agent-vps-ssh-tunnel.service
systemctl --user is-active btc-agent-immutable.service
systemctl --user is-active btc-agent-immutable-observe.timer
systemctl --user is-active btc-agent-immutable-backup.timer
systemctl --user is-active btc-agent-immutable-daily-check.timer
/home/admin/btc-agent/immutable/verify-runtime.sh
```

Expected:

```text
ssh: active
failed units: 0
tailscaled: active
btc-agent-vps-ssh-tunnel: active
btc-agent-immutable: active
immutable runtime verifier: PASS
```

The `admin` user has `Linger=yes`; the SSH tunnel and immutable runtime are user
units. The legacy WebUI tunnel and scheduler units are removed and must not be
recreated.

## Access service ownership

```text
~/.config/systemd/user/btc-agent-vps-ssh-tunnel.service
  Cloudflare SSH tunnel
  origin: ssh://127.0.0.1:22
  enabled, Restart=always

~/.config/systemd/user/btc-agent-immutable.service
  Sole scheduler authority
  immutable/current/agent scheduler
  runtime/config.yaml
```

Do not restart `btc-agent-immutable.service` while repairing access unless a
separate trading-operations incident explicitly requires it.

## Reboot recovery sequence

1. Try `ssh vps-ssh`.
2. If it fails, use the provider console/serial console.
3. Check SSH listener:

   ```bash
   systemctl status ssh --no-pager
   ss -ltnp | grep -E ':22|:2222' || true
   ```

4. Check Tailscale:

   ```bash
   systemctl status tailscaled --no-pager
   tailscale status
   tailscale ip -4
   ```

5. Check user manager, SSH tunnel, and immutable runtime:

   ```bash
   loginctl show-user admin -p Linger -p State -p RuntimePath
   systemctl status user@1000.service --no-pager
   systemctl --user status btc-agent-vps-ssh-tunnel.service --no-pager
   systemctl --user status btc-agent-immutable.service --no-pager
   /home/admin/btc-agent/immutable/verify-runtime.sh
   ```

6. Read current-boot logs before restarting anything:

   ```bash
   journalctl -b --user -u btc-agent-vps-ssh-tunnel.service -n 120 --no-pager
   journalctl -b --user -u btc-agent-immutable.service -n 120 --no-pager
   ```

7. If the SSH tunnel is failed and logs identify it as the access failure, restart
   only that tunnel:

   ```bash
   systemctl --user restart btc-agent-vps-ssh-tunnel.service
   ```

8. Re-test from Termux:

   ```bash
   ssh vps-ssh 'hostname; uptime -p; systemctl --failed --no-pager'
   ```

## Dynamic DNS updater

Prepared files:

```text
~/bin/update-ssh-direct-ddns
~/.config/systemd/user/ssh-direct-ddns.service
~/.config/systemd/user/ssh-direct-ddns.timer
~/.config/ddns/cloudflare.env.example
```

The updater:

- compares public IPv4 from two independent sources;
- validates source agreement;
- creates or updates `ssh-direct.linhbot.xyz` as DNS-only;
- updates only when the address changes;
- fails closed on duplicate records or API errors;
- does not log the API token;
- is sandboxed with systemd restrictions.

The timer is intentionally disabled until a Cloudflare API token scoped to
`Zone / DNS / Edit` for `linhbot.xyz` is provisioned at:

```text
~/.config/ddns/cloudflare.env
```

Required variable names:

```bash
CF_API_TOKEN=...
CF_ZONE_ID=...
```

Never commit this file. Set mode `0600`.

Activation after token provisioning:

```bash
chmod 600 ~/.config/ddns/cloudflare.env
DDNS_DRY_RUN=true ~/bin/update-ssh-direct-ddns
systemctl --user daemon-reload
systemctl --user enable --now ssh-direct-ddns.timer
systemctl --user start ssh-direct-ddns.service
systemctl --user status ssh-direct-ddns.service --no-pager
systemctl --user list-timers | grep ssh-direct-ddns
```

Only after successful DNS resolution and SSH validation should the local
`vps-ssh` alias switch from the direct IP to `ssh-direct.linhbot.xyz`.
Always retain `vps-direct` as a rollback route.

## Known non-access warning

Hermes user services contain invalid double-percent CPU quotas:

```text
CPUQuota=25%%
CPUQuota=30%%
CPUQuota=75%%
```

Systemd ignores these values. The correction to single `%` was verified offline
but is a separate runtime change and must not be mixed with access recovery.

## Security and trading boundaries

- Never store passwords, API tokens, Cloudflare credentials, or private keys in
  this repository.
- Do not print credential file contents into logs or chat.
- Do not restart the scheduler to repair SSH/tunnels.
- Do not enable live trading, resume operator halt, mutate production DB, or
  place/cancel orders during connectivity work.
- Preserve direct SSH as emergency access before changing DNS/tunnel policy.
