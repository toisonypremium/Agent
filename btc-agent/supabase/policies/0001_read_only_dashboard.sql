-- Authenticated dashboard users may read monitoring data. No client write policy.
do $$ declare t text; begin
  foreach t in array array['bot_instances','bot_heartbeats','agent_decisions','capital_plans','order_intents','exchange_orders','order_events','fills','positions','balance_snapshots','runtime_alerts','report_artifacts','audit_logs','bot_commands'] loop
    execute format('alter table public.%I enable row level security',t);
    execute format('drop policy if exists authenticated_read on public.%I',t);
    execute format('create policy authenticated_read on public.%I for select to authenticated using (true)',t);
  end loop;
end $$;
