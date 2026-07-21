#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

set -a
[ -f .env ] && source .env
set +a

tracked_secret_files="$(git ls-files | grep -E '(^|/)(\.env($|\.)|config\.ya?ml$|config\.local\.ya?ml$|secrets\.env$)|\.(db|sqlite)(-|$)|(^|/)(reports|logs|backups|bin)/' | grep -vE '(^|/)(config\.yaml\.example|btc-agent\.env\.example|deploy/cloudflared/config\.yml)$' || true)"
if [[ -n "$tracked_secret_files" ]]; then
  printf 'forbidden tracked runtime/secret files:\n%s\n' "$tracked_secret_files" >&2
  exit 1
fi

if git grep -n -I -E '(BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|sk-[A-Za-z0-9_-]{20,}|xox[baprs]-[A-Za-z0-9-]{20,})' -- . ':!scripts/check-secrets.sh'; then
  echo 'credential-like material found in tracked files' >&2
  exit 1
fi

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
