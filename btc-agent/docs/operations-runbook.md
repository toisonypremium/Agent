# Operations Runbook

Normal start: validate external config, start in paper/shadow, verify heartbeat,
reconcile OKX and inspect halt and ownership. Never enable execution
without a current reconcile and valid fencing token. During cloud outage keep local
risk/reconcile running. During OKX ambiguity halt new execution,
preserve client IDs and reconcile; never blind-retry with a new ID.
