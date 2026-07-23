# Environment Variables

Runtime secrets are loaded from the selected immutable service environment file.
Required values include the OKX key/secret/passphrase, Telegram token, and optional
LLM URL/key. Never place credentials in source control or browser-accessible variables.
Execution remains disabled unless explicit runtime/config gates and ownership pass.
