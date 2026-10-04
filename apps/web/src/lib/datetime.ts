// Formatting for the two kinds of time the API returns. They need opposite
// handling, and mixing them up is how a date silently slips a day.
//
// A `timestamptz` column (created_at, last_price_updated_at, profile_fetched_at)
// is an *instant*. Postgres always stores it as UTC, and it names one moment in
// time - so rendering it in the viewer's own timezone is both correct and what
// they expect: "9:27 PM" is a more useful answer to "when was this quoted" than
// "04:27 UTC". Use formatDateTime.
//
// A `date` column (a trade date, a payment date, a residency start) is a
// *calendar date* - no time, no timezone, nothing to convert. The API
// serializes it as UTC midnight, so formatting it in any zone west of UTC rolls
// it back a day: 2026-10-04 renders as Oct 3 in California. These are formatted
// in UTC, which is not a claim that the date "is UTC" - it's how you read back
// the same calendar date that was written. Use formatDate.

// formatDate renders a date-only value, in UTC so the calendar date survives
// the trip. For `date` columns only.
export const formatDate = (iso: string) =>
  new Date(iso).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    timeZone: 'UTC',
  })

// formatDateTime renders an instant in the viewer's timezone, with the zone
// named so there's no guessing which one it was converted to. For `timestamptz`
// columns only.
export const formatDateTime = (iso: string) =>
  new Date(iso).toLocaleString('en-US', {
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    timeZoneName: 'short',
  })

// todayLocal is the viewer's current calendar date as YYYY-MM-DD, for sending
// to a `date` column.
//
// It deliberately does not go through UTC. "Today" means today where the person
// is: at 9pm in California it is already tomorrow in UTC, so a UTC-derived
// "today" would record a date the user hasn't reached yet.
export function todayLocal(): string {
  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
}
