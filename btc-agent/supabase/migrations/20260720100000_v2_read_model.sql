-- V2 cloud read model. Exchange state remains authoritative in OKX.
create extension if not exists pgcrypto;

create table if not exists public.bot_instances (
  id uuid primary key,
  instance_name text not null unique,
  git_sha text not null,
  run_mode text not null check (run_mode in ('paper','shadow','live')),
  execution_enabled boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
create table if not exists public.bot_heartbeats (
  id uuid primary key default gen_random_uuid(), instance_id uuid not null references public.bot_instances(id),
  correlation_id uuid, fencing_token bigint, execution_owner boolean not null default false,
  scheduler_status text not null, last_analysis timestamptz, last_reconciliation timestamptz,
  last_exchange_success timestamptz, outbox_pending integer not null default 0 check(outbox_pending>=0),
  last_error text, payload jsonb not null default '{}'::jsonb, created_at timestamptz not null default now()
);
create index if not exists bot_heartbeats_instance_created_idx on public.bot_heartbeats(instance_id,created_at desc);

create table if not exists public.agent_decisions (
  id uuid primary key, instance_id uuid references public.bot_instances(id), correlation_id uuid not null,
  symbol text not null, decision text not null, confidence numeric check(confidence between 0 and 1),
  reasoning_summary text, payload jsonb not null, decided_at timestamptz not null,
  created_at timestamptz not null default now(), unique(instance_id,correlation_id,symbol)
);
create index if not exists agent_decisions_symbol_decided_idx on public.agent_decisions(symbol,decided_at desc);
create table if not exists public.capital_plans (
  id uuid primary key, instance_id uuid references public.bot_instances(id), correlation_id uuid not null,
  decision_id uuid references public.agent_decisions(id), state text not null,
  total_capital numeric not null check(total_capital>=0), risk_budget numeric not null check(risk_budget>=0),
  payload jsonb not null, planned_at timestamptz not null, created_at timestamptz not null default now(),
  unique(instance_id,correlation_id)
);
create table if not exists public.order_intents (
  id uuid primary key, instance_id uuid references public.bot_instances(id), correlation_id uuid not null,
  decision_id uuid references public.agent_decisions(id), plan_id uuid references public.capital_plans(id),
  idempotency_key text not null unique, client_order_id text not null unique, symbol text not null,
  side text not null check(side in ('BUY','SELL')), order_type text not null check(order_type in ('LIMIT','POST_ONLY')),
  price numeric not null check(price>0), quantity numeric not null check(quantity>0),
  status text not null, fencing_token bigint not null, payload jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(), updated_at timestamptz not null default now()
);
create index if not exists order_intents_symbol_created_idx on public.order_intents(symbol,created_at desc);
create table if not exists public.exchange_orders (
  id uuid primary key default gen_random_uuid(), intent_id uuid not null references public.order_intents(id),
  exchange_order_id text not null unique, status text not null, filled_quantity numeric not null default 0 check(filled_quantity>=0),
  average_price numeric check(average_price is null or average_price>0), exchange_updated_at timestamptz,
  payload jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), updated_at timestamptz not null default now()
);
create table if not exists public.order_events (
  id uuid primary key, intent_id uuid not null references public.order_intents(id), correlation_id uuid not null,
  event_type text not null, payload jsonb not null default '{}'::jsonb, occurred_at timestamptz not null,
  created_at timestamptz not null default now(), unique(intent_id,event_type,occurred_at)
);
create table if not exists public.fills (
  id uuid primary key, exchange_order_id uuid not null references public.exchange_orders(id),
  exchange_fill_id text not null unique, quantity numeric not null check(quantity>0), price numeric not null check(price>0),
  fee numeric not null default 0, fee_currency text, filled_at timestamptz not null, created_at timestamptz not null default now()
);
create table if not exists public.positions (
  id uuid primary key, instance_id uuid references public.bot_instances(id), symbol text not null,
  quantity numeric not null, average_entry numeric, unrealized_pnl numeric, reconciled_at timestamptz not null,
  payload jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), updated_at timestamptz not null default now(),
  unique(instance_id,symbol)
);
create table if not exists public.balance_snapshots (
  id uuid primary key, instance_id uuid references public.bot_instances(id), currency text not null,
  available numeric not null, total numeric not null, captured_at timestamptz not null,
  payload jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), unique(instance_id,currency,captured_at)
);
create table if not exists public.runtime_alerts (
  id uuid primary key, instance_id uuid references public.bot_instances(id), correlation_id uuid,
  severity text not null check(severity in ('INFO','WARNING','ERROR','CRITICAL')), category text not null,
  message text not null, acknowledged_at timestamptz, created_at timestamptz not null default now()
);
create index if not exists runtime_alerts_created_idx on public.runtime_alerts(created_at desc);
create table if not exists public.report_artifacts (
  id uuid primary key, instance_id uuid references public.bot_instances(id), correlation_id uuid,
  report_type text not null, object_key text not null unique, checksum text not null, content_type text not null,
  size_bytes bigint not null check(size_bytes>=0), created_at timestamptz not null default now()
);
create table if not exists public.audit_logs (
  id uuid primary key, instance_id uuid references public.bot_instances(id), correlation_id uuid,
  actor_id uuid, action text not null, resource_type text not null, resource_id text,
  details jsonb not null default '{}'::jsonb, created_at timestamptz not null default now()
);
create index if not exists audit_logs_created_idx on public.audit_logs(created_at desc);

-- Commands exist for schema evolution, but UI remains read-only until approval controls land.
create table if not exists public.bot_commands (
  id uuid primary key default gen_random_uuid(), created_by uuid not null references auth.users(id),
  command_type text not null, payload jsonb not null default '{}'::jsonb,
  status text not null default 'PENDING' check(status in ('PENDING','APPROVAL_REQUIRED','APPROVED','PROCESSING','COMPLETED','FAILED','EXPIRED')),
  approval_state text not null default 'NONE', expires_at timestamptz not null,
  processing_instance_id uuid references public.bot_instances(id), result jsonb, failure_reason text,
  created_at timestamptz not null default now(), updated_at timestamptz not null default now()
);

-- Read model security: service_role bypasses RLS; authenticated dashboard users are read-only.
alter table public.bot_instances enable row level security;
alter table public.bot_heartbeats enable row level security;
alter table public.agent_decisions enable row level security;
alter table public.capital_plans enable row level security;
alter table public.order_intents enable row level security;
alter table public.exchange_orders enable row level security;
alter table public.order_events enable row level security;
alter table public.fills enable row level security;
alter table public.positions enable row level security;
alter table public.balance_snapshots enable row level security;
alter table public.runtime_alerts enable row level security;
alter table public.report_artifacts enable row level security;
alter table public.audit_logs enable row level security;
alter table public.bot_commands enable row level security;

do $$
declare t text;
begin
  foreach t in array array['bot_instances','bot_heartbeats','agent_decisions','capital_plans','order_intents','exchange_orders','order_events','fills','positions','balance_snapshots','runtime_alerts','report_artifacts','audit_logs'] loop
    execute format('drop policy if exists dashboard_authenticated_read on public.%I', t);
    execute format('create policy dashboard_authenticated_read on public.%I for select to authenticated using (true)', t);
  end loop;
end $$;

create index if not exists positions_reconciled_idx on public.positions(reconciled_at desc);
create index if not exists order_intents_created_idx on public.order_intents(created_at desc);
create index if not exists report_artifacts_created_idx on public.report_artifacts(created_at desc);

insert into public.bot_instances(id,instance_name,git_sha,run_mode,execution_enabled)
values ('7f5e718b-0000-4000-8000-000000000001','v2-prod-01','09bd78e','shadow',false)
on conflict(instance_name) do update set git_sha=excluded.git_sha,run_mode='shadow',execution_enabled=false,updated_at=now();

create or replace view public.dashboard_operational_summary with (security_invoker=true) as
select i.id,i.instance_name,i.git_sha,i.run_mode,i.execution_enabled,
 h.scheduler_status,h.fencing_token,h.execution_owner,h.outbox_pending,h.last_error,h.created_at as heartbeat_at
from public.bot_instances i
left join lateral (select * from public.bot_heartbeats x where x.instance_id=i.id order by x.created_at desc limit 1) h on true;
create or replace view public.dashboard_recent_alerts with (security_invoker=true) as
select id,instance_id,severity,category,message,acknowledged_at,created_at from public.runtime_alerts order by created_at desc limit 200;
create or replace view public.dashboard_positions with (security_invoker=true) as
select id,instance_id,symbol,quantity,average_entry,unrealized_pnl,reconciled_at,updated_at from public.positions;
create or replace view public.dashboard_recent_decisions with (security_invoker=true) as
select id,instance_id,correlation_id,symbol,decision,confidence,reasoning_summary,decided_at from public.agent_decisions order by decided_at desc limit 200;
create or replace view public.dashboard_recent_orders with (security_invoker=true) as
select id,instance_id,correlation_id,client_order_id,symbol,side,order_type,price,quantity,status,fencing_token,created_at,updated_at from public.order_intents order by created_at desc limit 200;
create or replace view public.dashboard_artifacts with (security_invoker=true) as
select id,instance_id,correlation_id,report_type,object_key,checksum,content_type,size_bytes,created_at from public.report_artifacts order by created_at desc limit 200;

grant select on public.dashboard_operational_summary,public.dashboard_recent_alerts,public.dashboard_positions,public.dashboard_recent_decisions,public.dashboard_recent_orders,public.dashboard_artifacts to authenticated;
revoke all on public.bot_commands from anon,authenticated;
