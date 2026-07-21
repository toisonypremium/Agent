-- Additive LLM usage read model. Prompts, responses and credentials are never stored.
create table if not exists public.llm_usage_events (
  request_id text primary key,
  timestamp timestamptz not null,
  purpose text not null,
  trigger_source text,
  trigger_reason text,
  model text not null,
  prompt_tokens integer check(prompt_tokens is null or prompt_tokens >= 0),
  completion_tokens integer check(completion_tokens is null or completion_tokens >= 0),
  total_tokens integer check(total_tokens is null or total_tokens >= 0),
  usage_available boolean not null,
  latency_ms bigint not null check(latency_ms >= 0),
  status text not null check(status in ('ok','error','skipped')),
  error_class text,
  state_hash text,
  created_at timestamptz not null default now()
);
create index if not exists llm_usage_timestamp_idx on public.llm_usage_events(timestamp desc);
create index if not exists llm_usage_purpose_timestamp_idx on public.llm_usage_events(purpose,timestamp desc);
alter table public.llm_usage_events enable row level security;
drop policy if exists dashboard_authenticated_read on public.llm_usage_events;
create policy dashboard_authenticated_read on public.llm_usage_events for select to authenticated using (true);
revoke all on public.llm_usage_events from anon, authenticated;
grant select on public.llm_usage_events to authenticated;

create or replace view public.dashboard_llm_usage_daily with (security_invoker=true) as
select date_trunc('day', timestamp) as usage_day,
       purpose,
       model,
       count(*) filter (where status <> 'skipped') as calls,
       count(*) filter (where status = 'skipped') as skipped_calls,
       count(*) filter (where status = 'error') as failed_calls,
       count(*) filter (where status <> 'skipped' and not usage_available) as usage_unavailable,
       coalesce(sum(prompt_tokens),0) as prompt_tokens,
       coalesce(sum(completion_tokens),0) as completion_tokens,
       coalesce(sum(total_tokens),0) as total_tokens
from public.llm_usage_events
group by 1,2,3;
revoke all on public.dashboard_llm_usage_daily from anon, authenticated;
grant select on public.dashboard_llm_usage_daily to authenticated;
