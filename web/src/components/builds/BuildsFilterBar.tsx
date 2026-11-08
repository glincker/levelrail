import { useTranslation } from 'react-i18next'
import type { BuildRow } from '../../queries/buildCenter'

export type BuildStatusFilter =
  'all' | 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled'

export interface BuildsFilter {
  status: BuildStatusFilter
  query: string
}

export const EMPTY_FILTER: BuildsFilter = { status: 'all', query: '' }

const STATUSES: BuildStatusFilter[] = [
  'all',
  'queued',
  'running',
  'succeeded',
  'failed',
  'canceled',
]

// Held builds are waiting like queued ones, so they share the queued filter.
function matchesStatus(row: BuildRow, status: BuildStatusFilter): boolean {
  if (status === 'all') return true
  if (status === 'queued')
    return row.status === 'queued' || row.status === 'held'
  return row.status === status
}

export function filterBuilds(
  rows: BuildRow[],
  filter: BuildsFilter,
): BuildRow[] {
  const q = filter.query.trim().toLowerCase()
  return rows.filter(
    (r) =>
      matchesStatus(r, filter.status) &&
      (q === '' ||
        r.app.toLowerCase().includes(q) ||
        (r.branch ?? '').toLowerCase().includes(q) ||
        (r.commit_sha ?? '').toLowerCase().startsWith(q)),
  )
}

const FIELD =
  'h-8 rounded-md border border-border bg-background px-2 text-sm text-foreground'

interface Props {
  value: BuildsFilter
  onChange: (next: BuildsFilter) => void
}

export function BuildsFilterBar({ value, onChange }: Props) {
  const { t } = useTranslation('buildCenter')
  return (
    <div
      className="flex flex-wrap items-center gap-3"
      role="search"
      aria-label={t('filter.label')}
    >
      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        {t('filter.status')}
        <select
          className={FIELD}
          value={value.status}
          onChange={(e) => {
            onChange({
              ...value,
              status: e.target.value as BuildStatusFilter,
            })
          }}
        >
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {t(`filter.options.${s}`)}
            </option>
          ))}
        </select>
      </label>
      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        {t('filter.search')}
        <input
          type="search"
          className={`${FIELD} w-56`}
          value={value.query}
          placeholder={t('filter.placeholder')}
          onChange={(e) => {
            onChange({ ...value, query: e.target.value })
          }}
        />
      </label>
    </div>
  )
}
