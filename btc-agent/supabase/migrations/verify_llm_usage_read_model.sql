-- Read-only verification for the additive LLM usage read model.
select count(*) = 1 as llm_usage_table_exists
from information_schema.tables
where table_schema = 'public' and table_name = 'llm_usage_events';

select count(*) = 14 as llm_usage_column_count
from information_schema.columns
where table_schema = 'public' and table_name = 'llm_usage_events'
  and column_name in ('request_id','timestamp','purpose','trigger_source','trigger_reason','model','prompt_tokens','completion_tokens','total_tokens','usage_available','latency_ms','status','error_class','state_hash');

select c.relrowsecurity as llm_usage_rls_enabled
from pg_class c join pg_namespace n on n.oid = c.relnamespace
where n.nspname = 'public' and c.relname = 'llm_usage_events';

select count(*) >= 1 as authenticated_read_policy_exists
from pg_policies
where schemaname = 'public' and tablename = 'llm_usage_events'
  and roles::text like '%authenticated%' and cmd = 'SELECT';

select count(*) = 1 as daily_view_exists
from information_schema.views
where table_schema = 'public' and table_name = 'dashboard_llm_usage_daily';

select has_table_privilege('authenticated', 'public.llm_usage_events', 'SELECT') as authenticated_table_select,
       not has_table_privilege('authenticated', 'public.llm_usage_events', 'INSERT,UPDATE,DELETE') as authenticated_table_write_revoked,
       not has_table_privilege('anon', 'public.llm_usage_events', 'SELECT,INSERT,UPDATE,DELETE') as anon_table_access_revoked,
       has_table_privilege('authenticated', 'public.dashboard_llm_usage_daily', 'SELECT') as authenticated_view_select,
       not has_table_privilege('anon', 'public.dashboard_llm_usage_daily', 'SELECT') as anon_view_select_revoked;

select count(*) >= 6 as existing_dashboard_views_preserved
from information_schema.views
where table_schema = 'public' and table_name in ('dashboard_operational_summary','dashboard_recent_alerts','dashboard_positions','dashboard_recent_decisions','dashboard_recent_orders','dashboard_artifacts');
