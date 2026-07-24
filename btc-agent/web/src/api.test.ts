import { describe, expect, it, vi } from 'vitest'
import { readOverview, readPaperOrdersFiltered } from './api'

describe('readOverview', () => {
  it('uses the read-only overview endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ schema_version: 1, data: {}, freshness: {}, warnings: [] }) })
    vi.stubGlobal('fetch', fetchMock)
    await readOverview()
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/overview', expect.objectContaining({ credentials: 'same-origin' }))
  })
  it('uses an allowlisted paper status filter', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ schema_version: 1, data: {}, freshness: {}, warnings: [] }) })
    vi.stubGlobal('fetch', fetchMock)
    await readPaperOrdersFiltered('OPEN')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/paper/orders?limit=10&status=OPEN', expect.anything())
  })
  it('rejects unavailable API responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }))
    await expect(readOverview()).rejects.toThrow('503')
  })
})
