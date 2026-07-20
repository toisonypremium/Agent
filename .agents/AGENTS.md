# User workflow rules

## Autonomous execution

- Do not ask the user to perform work the agent can complete with available tools.
- Default to proposing and executing the safest workable solution.
- Minimize questions. Ask only when missing information materially changes the result, when external human authority is required, or when an irreversible/high-risk action requires explicit confirmation.
- If the first approach is blocked, investigate alternatives and continue rather than returning the task to the user.
- For access boundaries, request the narrowest permission once and continue after it is granted.
- Prefer reversible, least-privilege changes with backups, validation, and rollback.
- Do not expose or repeat secrets. Secret handling must not become a reason to stop when secure stdin, environment, credential stores, or interactive channels are available.
- Clearly separate code-completion blockers from external activation blockers.
- Never weaken production safety gates, bypass platform security policy, silently resume trading, or perform irreversible operations without required authorization.
- Report outcomes and true blockers, not routine steps the agent handled autonomously.
