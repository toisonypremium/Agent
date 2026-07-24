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
export type OKXAsset = { ma_tai_san: string; kha_dung: string; dang_khoa: string; tong: string; trang_thai_gan_thesis: string }
export type OKXAssets = { nguon?: string; thoi_diem_quan_sat?: string; trang_thai: string; tai_san: OKXAsset[]; canh_bao: string[] }

const apiBase = import.meta.env.VITE_WEB_CONSOLE_API_BASE ?? ''
async function read<T>(path: string, signal?: AbortSignal): Promise<Envelope<T>> {
  const response = await fetch(`${apiBase}${path}`, { credentials: 'same-origin', headers: { Accept: 'application/json' }, signal })
  if (!response.ok) throw new Error(`${path} request failed: ${response.status}`)
  return response.json() as Promise<Envelope<T>>
}
export const readOverview = (signal?: AbortSignal) => read<Overview>('/api/v1/overview', signal)
export const readScorecard = (signal?: AbortSignal) => read<Scorecard>('/api/v1/paper/scorecard', signal)
export const readPaperOrders = (signal?: AbortSignal) => read<{ orders: PaperOrder[]; limit: number }>('/api/v1/paper/orders?limit=10', signal)
export const readPaperOrdersFiltered = (status: string, signal?: AbortSignal) => {
  const query = new URLSearchParams({ limit: '10' })
  if (status) query.set('status', status)
  return read<{ orders: PaperOrder[]; limit: number }>(`/api/v1/paper/orders?${query}`, signal)
}
export type MarketPlanner = { available: boolean; generated_at?: string; price_usdt?: number; regime?: string; permission?: string; permission_reason?: string; risk_level?: string; falling_knife_risk?: string; fomo_risk?: string; market_summary?: string; plan_state?: string; plan_summary?: string; warnings: string[] }
export const readMarketPlanner = (signal?: AbortSignal) => read<MarketPlanner>('/api/v1/market-planner', signal)
export type AuditEvent = { id: number; timestamp: string; actor: string; action: string; result: string; request_id: string }
export type ReportCatalog = { reports: { id: string; title: string; generated_at: string }[] }
export const readReports = (signal?: AbortSignal) => read<ReportCatalog>('/api/v1/reports', signal)
export const readAudit = (signal?: AbortSignal) => read<{ events: AuditEvent[]; limit: number }>('/api/v1/audit?limit=12', signal)
export const readEvents = (signal?: AbortSignal) => read<{ events: Event[]; limit: number }>('/api/v1/events?limit=6', signal)
export const readRuntimeHealth = (signal?: AbortSignal) => read<RuntimeHealth>('/api/v1/operations/health', signal)
export const readCapitalOverview = (signal?: AbortSignal) => read<CapitalOverview>('/api/v1/capital/overview', signal)
export const readOKXAssets = (signal?: AbortSignal) => read<OKXAssets>('/api/v1/assets/okx', signal)
export const readThesisCapital = (signal?: AbortSignal) => read<{ items: ThesisCapital[]; limit: number }>('/api/v1/capital/theses?limit=8', signal)

export async function requestHalt(reasonCode: string, summary: string, signal?: AbortSignal): Promise<void> {
  const csrf = await read<{ csrf_token: string }>('/api/v1/csrf', signal)
  const response = await fetch(`${apiBase}/api/v1/halt`, {
    method: 'POST', credentials: 'same-origin', signal,
    headers: { Accept: 'application/json', 'Content-Type': 'application/json', 'X-CSRF-Token': csrf.data.csrf_token, 'Idempotency-Key': crypto.randomUUID() },
    body: JSON.stringify({ reason_code: reasonCode, summary }),
  })
  if (!response.ok) throw new Error(`halt request failed: ${response.status}`)
}
