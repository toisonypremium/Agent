#!/usr/bin/env bash
# Deprecated: user-level V2 service verification is no longer a production gate.
# Use docs/production-verification-checklist.md with immutable agent.service.
set -euo pipefail
printf '%s\n' 'btc-agent-v2 user service verification is retired; verify agent.service with the production checklist' >&2
exit 64
