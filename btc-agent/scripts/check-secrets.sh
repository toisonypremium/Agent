#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

set -a
[ -f .env ] && source .env
set +a

python3 - <<'PY'
import os
import re
from pathlib import Path

text = Path("config.yaml").read_text() if Path("config.yaml").exists() else ""

def val(k):
    m = re.search(r'^  ' + re.escape(k) + r':\s*["\']?(.*?)["\']?\s*$', text, re.M)
    return (m.group(1).strip() if m else "")

tok = val("telegram_token")
chat = val("telegram_chat_id")
print("telegram_token_set:", bool(tok))
print("telegram_token_has_ellipsis:", "…" in tok or "..." in tok)
print("telegram_chat_id_set:", bool(chat))
for k in ["OKX_API_KEY", "OKX_API_SECRET", "OKX_API_PASSPHRASE"]:
    v = os.getenv(k, "")
    print(k + "_set:", bool(v))
    print(k + "_has_ellipsis:", "…" in v or "..." in v)
PY
