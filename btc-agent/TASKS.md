# Tasks

## Open

- [ ] Initialize git repository and create baseline commit after explicit user approval.
- [ ] Use `make verify` before reporting implementation work done.
- [ ] Finish bootstrap identity cleanup only if `BOOTSTRAP.md` exists and user explicitly asks.

## Done

- [x] Add repository hygiene docs and verification gate.
- [x] Keep local config, data, reports, logs, backups, and binaries out of version control through `.gitignore`.

## Verification commands

```bash
make verify
git status --short --ignored
```

## Safety invariants

- No autonomous real trading by default.
- No futures.
- No leverage.
- Spot-limit-only execution path.
- Deterministic engine remains authority.
- `config.yaml` and secrets stay local-only.
