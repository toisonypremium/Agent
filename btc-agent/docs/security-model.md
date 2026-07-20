# Security Model

Frontend receives only Supabase URL and anon key. Supabase service-role, OKX,
Telegram and R2 secrets stay on the VPS under `/etc/agent/agent.env`. The website
never contacts OKX. Bot commands are disabled in the initial UI; dangerous commands
require expiry, approval/challenge, audit and execution-owner validation before a
future rollout. Logs must redact credentials and authorization headers.
