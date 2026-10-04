import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getJurisdictions, getTaxProfiles, putTaxProfile } from '../api/tax'
import type { TaxJurisdiction } from '../api/tax'
import { getUsers } from '../api/users'
import { formatDate, todayLocal } from '../lib/datetime'

// Mirrors tax.DefaultProfile on the API side: the residency assumed for a user
// with no profile saved. Preselected so making it explicit is one click, and
// labelled as a fallback until it's actually been saved.
const FALLBACK_COUNTRY = 'US'
const FALLBACK_REGION = 'US-CA'

const selectClasses =
  'w-full px-3 py-2.5 bg-surface-container-low border border-outline-variant rounded-lg text-body-md text-on-surface focus:outline-none focus:ring-2 focus:ring-secondary/30 focus:border-secondary transition-colors disabled:opacity-50'

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="block text-label-caps text-on-surface-variant uppercase mb-1.5">{label}</span>
      {children}
      {hint && <span className="block text-label-sm text-on-surface-variant mt-1.5">{hint}</span>}
    </label>
  )
}

// A settings group: a titled card with a description and whatever controls it
// owns. More sections will hang off the Settings page, so the frame is shared.
function SettingsSection({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <div className="bg-surface-container-lowest rounded-xl shadow-card">
      <div className="px-6 py-5 border-b border-outline-variant">
        <h2 className="text-headline-sm text-on-surface">{title}</h2>
        <p className="text-body-md text-on-surface-variant mt-1 max-w-2xl">{description}</p>
      </div>
      <div className="px-6 py-5">{children}</div>
    </div>
  )
}

// The residency form is a draft layered over what the server has: null means
// "show the saved values", and any edit fills it in. Deriving the displayed
// value this way (rather than copying server data into state with an effect)
// means a background refetch can't discard edits in progress, and there are no
// cascading renders to reason about.
interface Draft {
  country: string
  region: string
}

function TaxResidency() {
  const queryClient = useQueryClient()

  const usersQuery = useQuery({ queryKey: ['users'], queryFn: getUsers })
  const jurisdictionsQuery = useQuery({ queryKey: ['tax', 'jurisdictions'], queryFn: getJurisdictions })
  const profilesQuery = useQuery({ queryKey: ['tax', 'profiles'], queryFn: getTaxProfiles })

  // Null until someone picks, which resolves to the first user - so the
  // single-user case needs no interaction before the form means anything.
  const [selectedUserId, setSelectedUserId] = useState<string | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)

  const save = useMutation({
    mutationFn: ({ userId, country, region }: { userId: string; country: string; region: string }) =>
      putTaxProfile({
        user_id: userId,
        country_code: country,
        region_code: region || null,
        effective_from: todayLocal(),
      }),
    onSuccess: () => {
      // Drop the draft so the form follows the saved profile again.
      setDraft(null)
      queryClient.invalidateQueries({ queryKey: ['tax'] })
      // Treatments were rebuilt server-side, so every tax figure is stale.
      queryClient.invalidateQueries({ queryKey: ['holdings'] })
    },
  })

  const users = usersQuery.data ?? []
  const jurisdictions = jurisdictionsQuery.data ?? []
  const profiles = profilesQuery.data ?? []

  const userId = selectedUserId ?? users[0]?.id ?? ''
  const saved = profiles.find((p) => p.user_id === userId)

  // With no profile saved, the form shows the server's fallback; with one, it
  // shows exactly what's stored, where no region is a deliberate "none".
  const country = draft?.country ?? saved?.country_code ?? FALLBACK_COUNTRY
  const region = draft?.region ?? saved?.region_code ?? (saved ? '' : FALLBACK_REGION)

  const countries = jurisdictions.filter((j) => j.level === 'national')
  const regions = jurisdictions.filter((j) => j.level === 'regional' && j.parent_code === country)

  function pickUser(id: string) {
    setSelectedUserId(id)
    setDraft(null)
    save.reset()
  }

  function pickCountry(code: string) {
    // The old region belongs to the old country.
    setDraft({ country: code, region: '' })
    save.reset()
  }

  function pickRegion(code: string) {
    setDraft({ country, region: code })
    save.reset()
  }

  // A region the selected country doesn't own can't be submitted - it would
  // resolve to rules that don't exist.
  const regionValid = region === '' || regions.some((r) => r.code === region)
  const unchanged = saved != null && saved.country_code === country && (saved.region_code ?? '') === region
  const canSave = userId !== '' && country !== '' && regionValid && !unchanged && !save.isPending

  const loading = usersQuery.isLoading || jurisdictionsQuery.isLoading || profilesQuery.isLoading
  const loadError = usersQuery.error ?? jurisdictionsQuery.error ?? profilesQuery.error

  if (loading) return <p className="text-body-md text-on-surface-variant">Loading residency…</p>
  if (loadError) return <p className="text-body-md text-error">{(loadError as Error).message}</p>
  if (users.length === 0) {
    return <p className="text-body-md text-on-surface-variant">Add a user before setting a tax residency.</p>
  }

  return (
    <div className="space-y-5">
      {!saved && (
        <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-surface-container-high">
          <span className="material-symbols-outlined text-on-surface-variant text-xl flex-shrink-0">info</span>
          <p className="text-body-md text-on-surface-variant">
            No residency saved for this person yet, so the API is falling back to its default of{' '}
            <span className="font-semibold text-on-surface">United States / California</span>. Saving makes it explicit
            and stops the default from applying.
          </p>
        </div>
      )}

      <div className="grid gap-5 md:grid-cols-3 max-w-3xl">
        <Field label="Person">
          {users.length > 1 ? (
            <select value={userId} onChange={(e) => pickUser(e.target.value)} className={selectClasses}>
              {users.map((u) => (
                <option key={u.id} value={u.id}>{`${u.first_name} ${u.last_name}`}</option>
              ))}
            </select>
          ) : (
            <p className="px-3 py-2.5 text-body-md text-on-surface">{`${users[0].first_name} ${users[0].last_name}`}</p>
          )}
        </Field>

        <Field label="Country">
          <select value={country} onChange={(e) => pickCountry(e.target.value)} className={selectClasses}>
            {countries.map((j: TaxJurisdiction) => (
              <option key={j.code} value={j.code}>{j.name}</option>
            ))}
          </select>
        </Field>

        <Field
          label="State / Region"
          hint={
            regions.length === 0
              ? 'This country has no regional jurisdictions.'
              : 'Leave as None if not subject to a regional income tax.'
          }
        >
          <select
            value={region}
            onChange={(e) => pickRegion(e.target.value)}
            disabled={regions.length === 0}
            className={selectClasses}
          >
            <option value="">None</option>
            {regions.map((j) => (
              <option key={j.code} value={j.code}>{j.name}</option>
            ))}
          </select>
        </Field>
      </div>

      <div className="flex items-center gap-4">
        <button
          onClick={() => save.mutate({ userId, country, region })}
          disabled={!canSave}
          className="px-4 py-2.5 bg-primary text-on-primary rounded-lg text-body-md font-semibold hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          {save.isPending ? 'Saving…' : 'Save Residency'}
        </button>

        {save.isError ? (
          <p className="text-body-md text-error">{save.error.message}</p>
        ) : save.isSuccess ? (
          <p className="text-body-md text-secondary">Saved. Existing tax treatments were rebuilt.</p>
        ) : saved ? (
          <p className="text-body-md text-on-surface-variant">Effective from {formatDate(saved.effective_from)}.</p>
        ) : null}
      </div>

      <p className="text-label-sm text-on-surface-variant max-w-2xl">
        Residency decides which jurisdictions' rules every realized gain and distribution is evaluated against, so
        changing it recomputes the tax treatment of everything already realized.
      </p>
    </div>
  )
}

export default function Settings() {
  return (
    <div className="p-8">
      <div className="mb-8">
        <h1 className="text-headline-lg text-on-surface mb-2">Settings</h1>
        <p className="text-body-lg text-on-surface-variant max-w-2xl">
          How the portfolio is interpreted — who you are for tax purposes, and the rules that follow from it.
        </p>
      </div>

      <div className="space-y-6">
        <SettingsSection
          title="Tax Residency"
          description="Where you're taxed. A country, plus a state or region where the country taxes regionally."
        >
          <TaxResidency />
        </SettingsSection>
      </div>
    </div>
  )
}
