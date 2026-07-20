# Environment Variables

Runtime secrets are loaded from `/etc/agent/agent.env`; names are referenced by the
typed YAML config. Required groups include OKX key/secret/passphrase, Telegram token,
LLM URL/key, Supabase URL/service-role for the VPS adapter, and R2 endpoint/access
credentials. Frontend receives only `NEXT_PUBLIC_SUPABASE_URL` and
`NEXT_PUBLIC_SUPABASE_ANON_KEY`. Never prefix server secrets with `NEXT_PUBLIC_`.
Execution remains disabled unless explicit runtime/config gates and ownership pass.
