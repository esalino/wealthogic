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
