import { useId, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LightbulbGlassIcon } from '@thesvg/ui-react'
import { PageHeader } from '@/components/shell/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { requireExperimental } from '../lib/experimental'
import {
  advisorFindingsQueryOptions,
  useAdvisorFindings,
  useAdvisorSummary,
} from '../queries/resourceAdvisor'
import { splitPending } from '../lib/resourceAdvisor'
import { formatBytes } from '../lib/format'
import {
  FindingList,
  type FindingGrouping,
} from '../components/resourceAdvisor/FindingList'

export const Route = createFileRoute('/resource-advisor')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'resource-advisor', location.href),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(advisorFindingsQueryOptions()),
  component: ResourceAdvisorPage,
  pendingComponent: ResourceAdvisorSkeleton,
})

function ResourceAdvisorSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <Skeleton className="h-6 w-48" />
      <Skeleton className="h-4 w-96" />
      <Skeleton className="h-24 w-full rounded-lg" />
      <Skeleton className="h-24 w-full rounded-lg" />
    </div>
  )
}

function SummaryStrip() {
  const { t } = useTranslation('resourceAdvisor')
  const { data } = useAdvisorSummary()
  if (!data) return null
  return (
    <div className="flex flex-wrap gap-2 text-sm" data-testid="advisor-summary">
      <Badge variant="outline">
        {t('summary.findings', { count: data.total })}
      </Badge>
      {data.by_severity.warn ? (
        <Badge variant="warning">
          {t('summary.warnings', { count: data.by_severity.warn })}
        </Badge>
      ) : null}
      {data.actionable ? (
        <Badge variant="info">
          {t('summary.actionable', { count: data.actionable })}
        </Badge>
      ) : null}
      {data.saving_memory_bytes > 0 ? (
        <Badge variant="success">
          {t('summary.saving', {
            amount: formatBytes(data.saving_memory_bytes),
          })}
        </Badge>
      ) : null}
      {data.pending_data > 0 ? (
        <Badge variant="muted">
          {t('summary.pending', { count: data.pending_data })}
        </Badge>
      ) : null}
    </div>
  )
}

function ResourceAdvisorPage() {
  const { t } = useTranslation('resourceAdvisor')
  const [grouping, setGrouping] = useState<FindingGrouping>('resource')
  const [showHidden, setShowHidden] = useState(false)
  const hiddenId = useId()
  const { data, isError, error } = useAdvisorFindings('', showHidden)
  const { advice, pending } = splitPending(data?.findings ?? [])

  return (
    <div className="space-y-6">
      <PageHeader
        title={
          <span className="flex items-center gap-2">
            {t('title')}
            <Badge variant="outline" title={t('beta.hint')}>
              {t('beta.label')}
            </Badge>
          </span>
        }
        description={t('description')}
        helpPath="/resource-advisor"
        helpLabel={t('title')}
      />
      <SummaryStrip />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div
          className="flex gap-1"
          role="group"
          aria-label={t('groupBy.label')}
        >
          {(['resource', 'savings'] as const).map((g) => (
            <Button
              key={g}
              size="sm"
              variant={grouping === g ? 'secondary' : 'ghost'}
              aria-pressed={grouping === g}
              onClick={() => setGrouping(g)}
            >
              {t(`groupBy.${g}`)}
            </Button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id={hiddenId}
            checked={showHidden}
            onCheckedChange={setShowHidden}
          />
          <Label htmlFor={hiddenId}>{t('showHidden')}</Label>
        </div>
      </div>
      {isError ? (
        <p className="text-sm text-destructive">
          {t('error', { message: error.message })}
        </p>
      ) : advice.length === 0 ? (
        <EmptyState
          icon={<LightbulbGlassIcon size="1.2em" />}
          title={
            pending.length > 0 ? t('empty.pendingTitle') : t('empty.title')
          }
          description={
            pending.length > 0
              ? t('empty.pendingDescription')
              : t('empty.description')
          }
        />
      ) : (
        <FindingList findings={advice} grouping={grouping} />
      )}
      {pending.length > 0 && advice.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('summary.pending', { count: pending.length })}:{' '}
          {pending.map((f) => f.resource.name).join(', ')}
        </p>
      ) : null}
    </div>
  )
}
