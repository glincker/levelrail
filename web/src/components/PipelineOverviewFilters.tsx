import { useQuery } from '@tanstack/react-query'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { appListQueryOptions } from '../queries/apps'
import type {
  PipelineOverviewFilters as Filters,
  PipelineOverviewStatusFilter,
} from '../types/pipelineOverview'
import type { PipelineTrigger } from '../types/pipelines'

const ALL = 'all'

const STATUS_OPTIONS: { value: PipelineOverviewStatusFilter; label: string }[] =
  [
    { value: 'running', label: 'Running' },
    { value: 'failed', label: 'Failed' },
    { value: 'succeeded', label: 'Succeeded' },
    { value: 'cancelled', label: 'Cancelled' },
    { value: 'waiting_approval', label: 'Needs approval' },
    { value: 'held', label: 'Held' },
  ]

const TRIGGER_OPTIONS: PipelineTrigger[] = [
  'push',
  'pull_request',
  'tag',
  'manual',
  'schedule',
  'api',
]

export function PipelineOverviewFilters({
  filters,
  onChange,
}: {
  filters: Filters
  onChange: (next: Filters) => void
}) {
  const { data: apps } = useQuery(appListQueryOptions())
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        value={filters.status ?? ALL}
        onValueChange={(v) =>
          onChange({
            ...filters,
            status:
              v && v !== ALL ? (v as PipelineOverviewStatusFilter) : undefined,
          })
        }
      >
        <SelectTrigger className="w-40" aria-label="Status">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>All statuses</SelectItem>
          {STATUS_OPTIONS.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={filters.app ?? ALL}
        onValueChange={(v) =>
          onChange({ ...filters, app: v && v !== ALL ? v : undefined })
        }
      >
        <SelectTrigger className="w-40" aria-label="App">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>All apps</SelectItem>
          {(apps ?? []).map((a) => (
            <SelectItem key={a.name} value={a.name}>
              {a.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={filters.trigger ?? ALL}
        onValueChange={(v) =>
          onChange({
            ...filters,
            trigger: v && v !== ALL ? (v as PipelineTrigger) : undefined,
          })
        }
      >
        <SelectTrigger className="w-40" aria-label="Trigger">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>All triggers</SelectItem>
          {TRIGGER_OPTIONS.map((t) => (
            <SelectItem key={t} value={t}>
              {t}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        className="w-44"
        aria-label="Pipeline name"
        placeholder="Pipeline name"
        value={filters.pipeline ?? ''}
        onChange={(e) =>
          onChange({ ...filters, pipeline: e.target.value || undefined })
        }
      />
    </div>
  )
}
