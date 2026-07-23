#!/usr/bin/env bash
# Deprecated: do not install a user-level scheduler from a Git checkout.
# The supported unattended runtime is deploy/systemd/agent.service from an
# immutable /opt/agent/current release.
set -euo pipefail
printf '%s\n' 'btc-agent-v2 user service is retired; install and verify agent.service instead' >&2
exit 64
