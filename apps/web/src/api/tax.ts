const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface TaxJurisdiction {
  code: string
  name: string
  level: string // 'national' | 'regional'
  parent_code: string | null
  currency_code: string
}

// One taxable bucket within a jurisdiction. The label comes from the API so a
// jurisdiction added as data arrives readable, without a client-side mapping to
// keep in sync.
export interface TaxBucket {
  character: string
  label: string
  amount: number
}

// Income a jurisdiction did not tax, and why.
export interface TaxExclusion {
  reason: string
  label: string
  amount: number
}

export interface JurisdictionSummary {
  code: string
  name: string
  level: string
  taxable_total: number
  buckets: TaxBucket[]
  excluded: TaxExclusion[]
}

// The year's taxable income per jurisdiction. Jurisdictions differ in shape -
// one may split capital gains by holding period while another folds everything
// into ordinary income - so the client renders whatever buckets it's given
// rather than assuming a fixed set.
export interface TaxSummary {
  year: number
  jurisdictions: JurisdictionSummary[]
}

export async function getTaxSummary(year: number): Promise<TaxSummary> {
  const res = await fetch(`${API_BASE}/tax/summary?year=${year}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch tax summary')
  }

  return res.json()
}

export async function getJurisdictions(): Promise<TaxJurisdiction[]> {
  const res = await fetch(`${API_BASE}/tax/jurisdictions`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch jurisdictions')
  }

  return res.json()
}
