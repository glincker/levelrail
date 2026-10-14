import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DEPLOYMENT_STATUSES } from '../../types/deployment'
import {
  DEFAULT_TIMELINE_FILTERS,
  TIMELINE_ZOOMS,
  type TimelineFilters as Filters,
  type TimelineZoom,
} from '../../lib/deployTimeline'

export interface NamedOption {
  id: string
  name: string
}

interface Props {
  filters: Filters
  apps: string[]
  projects: NamedOption[]
  nodes: NamedOption[]
  onChange: (next: Filters) => void
}

const SELECT_CLASS =
  'h-9 min-w-0 rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

function FilterSelect({
  label,
  allLabel,
  value,
  options,
  onChange,
}: {
  label: string
  allLabel: string
  value: string
  options: NamedOption[]
  onChange: (v: string) => void
}) {
  return (
    <label className="flex flex-col gap-1 text-[11px] text-muted-foreground">
      {label}
      <select
        className={SELECT_CLASS}
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      >
        <option value="">{allLabel}</option>
        {options.map((o) => (
          <option key={o.id} value={o.id}>
            {o.name}
          </option>
        ))}
      </select>
    </label>
  )
}

export function TimelineFilters({
  filters,
  apps,
  projects,
  nodes,
  onChange,
}: Props) {
  const { t } = useTranslation('deployTimeline')
  const set = (patch: Partial<Filters>) => {
    onChange({ ...filters, ...patch })
  }
  const dirty =
    filters.app !== '' ||
    filters.project !== '' ||
    filters.node !== '' ||
    filters.status !== ''
  return (
    <div className="flex flex-wrap items-end gap-3">
      <div
        role="group"
        aria-label={t('zoom.label')}
        className="inline-flex rounded-md border border-border p-0.5"
      >
        {TIMELINE_ZOOMS.map((z: TimelineZoom) => (
          <button
            key={z}
            type="button"
            aria-pressed={filters.zoom === z}
            onClick={() => {
              set({ zoom: z })
            }}
            className="h-8 rounded px-3 text-sm text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring aria-pressed:bg-muted aria-pressed:text-foreground"
          >
            {t(`zoom.${z}`)}
          </button>
        ))}
      </div>
      <FilterSelect
        label={t('filters.app')}
        allLabel={t('filters.allApps')}
        value={filters.app}
        options={apps.map((a) => ({ id: a, name: a }))}
        onChange={(app) => {
          set({ app })
        }}
      />
      <FilterSelect
        label={t('filters.project')}
        allLabel={t('filters.allProjects')}
        value={filters.project}
        options={projects}
        onChange={(project) => {
          set({ project })
        }}
      />
      <FilterSelect
        label={t('filters.node')}
        allLabel={t('filters.allNodes')}
        value={filters.node}
        options={nodes}
        onChange={(node) => {
          set({ node })
        }}
      />
      <FilterSelect
        label={t('filters.status')}
        allLabel={t('filters.allStatuses')}
        value={filters.status}
        options={DEPLOYMENT_STATUSES.map((s) => ({
          id: s,
          name: t(`status.${s}`),
        }))}
        onChange={(status) => {
          set({ status })
        }}
      />
      {dirty ? (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            onChange({ ...DEFAULT_TIMELINE_FILTERS, zoom: filters.zoom })
          }}
        >
          {t('filters.clear')}
        </Button>
      ) : null}
    </div>
  )
}
