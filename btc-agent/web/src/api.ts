export type Freshness = { state: 'fresh' | 'stale' | 'unavailable'; age_seconds: number }
export type Envelope<T> = { schema_version: number; generated_at: string; freshness: Freshness; data: T; warnings: string[] }

export type Overview = {
  halted: boolean
  market: { available: boolean; regime?: string; permission?: string; plan_state?: string; accumulation_phase?: string; generated_at?: string }
  lease: { available: boolean; instance_id?: string; fencing_token?: number; expires_at?: string; fresh: boolean }
  paper: { total_orders: number; terminal_orders: number; readiness: string; unknown_statuses: number; missing_terminal_timestamps: number }
}

const apiBase = import.meta.env.VITE_WEB_CONSOLE_API_BASE ?? ''

export async function readOverview(signal?: AbortSignal): Promise<Envelope<Overview>> {
  const response = await fetch(`${apiBase}/api/v1/overview`, { credentials: 'same-origin', headers: { Accept: 'application/json' }, signal })
  if (!response.ok) throw new Error(`overview request failed: ${response.status}`)
  return response.json() as Promise<Envelope<Overview>>
}
