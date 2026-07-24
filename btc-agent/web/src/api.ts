export type Freshness = { state: 'fresh' | 'stale' | 'unavailable'; age_seconds: number }
export type Envelope<T> = { schema_version: number; generated_at: string; freshness: Freshness; data: T; warnings: string[] }

export type Overview = {
  halted: boolean
  market: { available: boolean; regime?: string; permission?: string; plan_state?: string; accumulation_phase?: string; generated_at?: string }
  lease: { available: boolean; instance_id?: string; fencing_token?: number; expires_at?: string; fresh: boolean }
  paper: { total_orders: number; terminal_orders: number; readiness: string; unknown_statuses: number; missing_terminal_timestamps: number }
}
export type Scorecard = { total_orders: number; open_orders: number; terminal_orders: number; filled_orders: number; invalidated_orders: number; expired_orders: number; cancelled_orders: number; readiness: string; blockers: string[]; unknown_statuses: number; missing_terminal_timestamps: number }
export type PaperOrder = { id: string; timestamp: string; symbol: string; side: string; layer: number; price: number; quantity: number; notional: number; status: string; expires_at: string; closed_at?: string; reason?: string }
export type Event = { id: number; timestamp: string; source: string; type: string; severity: string }
export type RuntimeHealth = { freshness: Freshness; scheduler: { state: string; count: number }; heartbeat: { state: string; age_seconds: number }; lease: { instance_id?: string; fresh: boolean }; database_state: string; observer_state: string }
export type CapitalOverview = { currency: 'USDT'; source_at: string; available_usdt: number; reserved_usdt: number; filled_usdt: number; max_exposure_usdt: number; projection_state: string; issues: string[] }
export type ThesisCapital = { thesis_id: string; symbol: string; status: string; max_exposure_usdt: number; reserved_usdt: number; filled_usdt: number; remaining_usdt: number; updated_at: string; blockers: string[] }

const apiBase = import.meta.env.VITE_WEB_CONSOLE_API_BASE ?? ''
async function read<T>(path: string, signal?: AbortSignal): Promise<Envelope<T>> {
  const response = await fetch(`${apiBase}${path}`, { credentials: 'same-origin', headers: { Accept: 'application/json' }, signal })
  if (!response.ok) throw new Error(`${path} request failed: ${response.status}`)
  return response.json() as Promise<Envelope<T>>
}
export const readOverview = (signal?: AbortSignal) => read<Overview>('/api/v1/overview', signal)
export const readScorecard = (signal?: AbortSignal) => read<Scorecard>('/api/v1/paper/scorecard', signal)
export const readPaperOrders = (signal?: AbortSignal) => read<{ orders: PaperOrder[]; limit: number }>('/api/v1/paper/orders?limit=10', signal)
export const readEvents = (signal?: AbortSignal) => read<{ events: Event[]; limit: number }>('/api/v1/events?limit=6', signal)
export const readRuntimeHealth = (signal?: AbortSignal) => read<RuntimeHealth>('/api/v1/operations/health', signal)
export const readCapitalOverview = (signal?: AbortSignal) => read<CapitalOverview>('/api/v1/capital/overview', signal)
export const readThesisCapital = (signal?: AbortSignal) => read<{ items: ThesisCapital[]; limit: number }>('/api/v1/capital/theses?limit=8', signal)

export async function requestHalt(reason: string, signal?: AbortSignal): Promise<void> {
  const csrf = await read<{ csrf_token: string }>('/api/v1/csrf', signal)
  const response = await fetch(`${apiBase}/api/v1/halt`, {
    method: 'POST', credentials: 'same-origin', signal,
    headers: { Accept: 'application/json', 'Content-Type': 'application/json', 'X-CSRF-Token': csrf.data.csrf_token, 'Idempotency-Key': crypto.randomUUID() },
    body: JSON.stringify({ reason }),
  })
  if (!response.ok) throw new Error(`halt request failed: ${response.status}`)
}
