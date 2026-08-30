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
