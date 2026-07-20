# Secret migration and rotation

1. Set `TELEGRAM_TOKEN`, `TELEGRAM_CHAT_ID`, OKX and LLM credentials only in the protected systemd environment file (`0600`, service owner only).
2. Remove `notify.telegram_token` from YAML. Startup now rejects a non-empty YAML token.
3. Restart in staging and run runtime doctor without printing values.
4. Rotate Telegram token through BotFather if plaintext existed in config, backup, terminal history, logs or Git history. Revoke the old token before production acceptance.
5. Audit backups/history using secret-name and entropy scans; never copy findings containing values into Git or reports.
6. Validate notifications, readiness and rollback. Keep trading authority blocked during migration.
