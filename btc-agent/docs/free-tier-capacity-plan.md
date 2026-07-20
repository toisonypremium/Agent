# Free-tier Capacity Plan

Initial estimate at one bot, four assets: heartbeat every minute (1,440 rows/day),
24 decision cycles/day (under 100 decision/plan rows/day), under 500 order events/day,
and fewer than 20 report objects/day. A compact heartbeat row around 1 KB consumes
about 45 MB/month before indexes. Retain detailed heartbeats 14 days, aggregate daily
health beyond that; retain alerts 30 days; retain decisions/plans 12 months; retain
orders, fills and audit long term; move report payloads to R2 and keep metadata only.
Dashboard uses Supabase realtime for current state and bounded latest queries, not
continuous polling. Review capacity monthly and reduce heartbeat cadence before any
free-tier overage.
