const API_BASE = import.meta.env.API_URL ?? 'http://localhost:8080'

export interface User {
  id: string
  first_name: string
  last_name: string
}

export async function getUsers(): Promise<User[]> {
  const res = await fetch(`${API_BASE}/users`)
  if (!res.ok) throw new Error('Failed to fetch users')
  return res.json()
}

export interface CreateUserPayload {
  first_name: string
  last_name: string
}

export async function createUser(payload: CreateUserPayload): Promise<User> {
  const res = await fetch(`${API_BASE}/users`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error ?? 'Failed to create user')
  }

  return res.json()
}
