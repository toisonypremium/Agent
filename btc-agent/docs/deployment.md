# VPS Deployment

The supported runtime is the unprivileged immutable user service described in
[immutable-deployment.md](immutable-deployment.md). It runs one scheduler from
`$HOME/btc-agent/immutable/current/agent`, with mutable state under
`$HOME/btc-agent/runtime`.

Do not run from a Git clone. Do not install a root-managed service, legacy V2
service, Termux loop, cron, PM2 process, or shell scheduler wrapper. Deployment
never clears operator halt or enables real execution.
