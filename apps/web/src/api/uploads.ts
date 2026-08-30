const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface UploadResult {
  created: number
  updated: number
  skipped: number
}

export interface UploadParams {
  file: File
  fileType: string // 'holdings' | 'transactions'
  accountType: string // institution, e.g. 'fidelity'
  accountId?: string
}

export interface Upload {
  id: string
  file_name: string
  start_date: string | null
  end_date: string | null
  account_id: string
  created_at: string
  updated_at: string
}

export interface UploadTransaction {
  id: string
  asset_type: string | null
  symbol: string
  asset_description: string | null
  action: string
  date: string
  quantity: number | null
  price: number | null
  amount: number
  commission: number
  fees: number
  settlement_date: string | null
  realized_gains: number
  transaction_id: string
  upload_id: string
  created_at: string
  updated_at: string
}

export interface PaginatedUploads {
  data: Upload[]
  total: number
  page: number
  page_size: number
}

export interface PaginatedUploadTransactions {
  data: UploadTransaction[]
  total: number
  page: number
  page_size: number
}

export async function getUploads(page = 1, pageSize = 20): Promise<PaginatedUploads> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  const res = await fetch(`${API_BASE}/uploads?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch uploads')
  }

  return res.json()
}

export async function getUploadTransactions(page = 1, pageSize = 20): Promise<PaginatedUploadTransactions> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  const res = await fetch(`${API_BASE}/upload-transactions?${params}`)

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to fetch upload transactions')
  }

  return res.json()
}

export async function uploadFile({ file, fileType, accountType, accountId }: UploadParams): Promise<UploadResult> {
  const form = new FormData()
  form.append('file', file)
  form.append('file_type', fileType)
  form.append('account_type', accountType)
  if (accountId) form.append('account_id', accountId)

  // No explicit Content-Type — the browser sets the multipart boundary.
  const res = await fetch(`${API_BASE}/uploads`, { method: 'POST', body: form })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to upload file')
  }

  return res.json()
}
