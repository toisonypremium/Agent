#!/bin/bash
set -euo pipefail
systemctl --user disable --now btc-agent-circuit-research.timer || true
systemctl --user stop btc-agent-circuit-research.service || true
echo 'Circuit sidecar disabled; evidence and logs preserved.'
