#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

reject_bad_value() {
  local name="$1"
  local value="$2"
  if [[ -z "$value" ]]; then
    echo "ERROR: $name is empty" >&2
    exit 1
  fi
  if [[ "$value" == *"…"* || "$value" == *"..."* ]]; then
    echo "ERROR: $name contains ellipsis; provide full value" >&2
    exit 1
  fi
}

shell_single_quote() {
  local value="$1"
  printf "'%s'" "${value//\'/\'\\\'\'}"
}

json_escape() {
  python3 -c 'import json, sys; print(json.dumps(sys.argv[1]))' "$1"
}

read_secret() {
  local prompt="$1"
  local value
  read -r -s -p "$prompt: " value
  echo
  printf '%s' "$value"
}

telegram_token=$(read_secret "Telegram bot token")
read -r -p "Telegram chat id [2015953678]: " telegram_chat_id
telegram_chat_id=${telegram_chat_id:-2015953678}
okx_api_key=$(read_secret "OKX API key")
okx_api_secret=$(read_secret "OKX API secret")
okx_api_passphrase=$(read_secret "OKX API passphrase")

reject_bad_value "Telegram bot token" "$telegram_token"
reject_bad_value "Telegram chat id" "$telegram_chat_id"
reject_bad_value "OKX API key" "$okx_api_key"
reject_bad_value "OKX API secret" "$okx_api_secret"
reject_bad_value "OKX API passphrase" "$okx_api_passphrase"

{
  printf 'export OKX_API_KEY=%s\n' "$(shell_single_quote "$okx_api_key")"
  printf 'export OKX_API_SECRET=%s\n' "$(shell_single_quote "$okx_api_secret")"
  printf 'export OKX_API_PASSPHRASE=%s\n' "$(shell_single_quote "$okx_api_passphrase")"
} > .env
chmod 600 .env

python3 - "$telegram_token" "$telegram_chat_id" <<'PY'
import json
import re
import sys
from pathlib import Path

token = sys.argv[1]
chat_id = sys.argv[2]
path = Path("config.yaml")
text = path.read_text()
block = "\n".join([
    "notify:",
    "  enabled: true",
    "  provider: \"telegram\"",
    "  telegram_token: " + json.dumps(token),
    "  telegram_chat_id: " + json.dumps(chat_id),
    "  ntfy_topic: \"\"",
])
pattern = r'(?ms)^notify:\n(?:^  .*\n?)*'
if re.search(pattern, text):
    text = re.sub(pattern, block + "\n", text, count=1)
else:
    text = text.rstrip() + "\n\n" + block + "\n"
path.write_text(text)
PY

python3 - <<'PY'
from pathlib import Path
path = Path('.gitignore')
text = path.read_text() if path.exists() else ''
lines = text.splitlines()
for item in ['.env', 'config.yaml', 'config.local.yaml', 'data/', 'reports/', 'bin/', 'logs/', '*.db']:
    if item not in lines:
        lines.append(item)
path.write_text('\n'.join(lines).rstrip() + '\n')
PY

if [ ! -f scripts/check-secrets.sh ]; then
  cat > scripts/check-secrets.sh <<'EOF'
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
EOF
fi

if [ ! -f scripts/load-env-and-run.sh ]; then
  cat > scripts/load-env-and-run.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi
exec ./bin/btc-agent "$@"
EOF
fi

chmod +x scripts/setup-secrets.sh scripts/check-secrets.sh scripts/load-env-and-run.sh

echo "Secret setup complete. Values hidden. Run scripts/check-secrets.sh to verify redacted status."
