# Agent Done Rules

Agents must not report `done` unless work is proven by files and commands.

## Required proof

Before claiming completion, agent must provide:

- changed files with short summaries;
- exact verification commands run;
- PASS/FAIL for each command;
- key output for failures or safety-relevant warnings;
- remaining real risks.

## Verification gate

Run:

```bash
make verify
```

Equivalent expanded commands:

```bash
go test -v -count=1 ./...
go vet ./...
go build -o bin/btc-agent .
./bin/btc-agent status --config config.yaml
./bin/btc-agent live-proof --config config.yaml
```

`live-proof` must run without panic and must not place any real order.

## Hard rule

If no source, test, or docs files changed, final answer must start exactly:

```text
NOT DONE - no source files changed
```

## Safety rules

- Do not add secrets.
- Do not print secrets in logs or final answers.
- Do not commit `config.yaml`, `config.local.yaml`, `.env`, databases, reports, logs, backups, or binaries.
- Do not enable autonomous real trading.
- Do not remove safety gates.
- Do not place real orders.
