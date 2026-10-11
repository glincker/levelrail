import { useTranslation } from 'react-i18next'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { ScheduledTaskRunStatus } from '../types/scheduledTasks'

const VARIANT: Record<
  ScheduledTaskRunStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  running: 'default',
  success: 'success',
  failed: 'destructive',
  timeout: 'warning',
  missed: 'warning',
  interrupted: 'warning',
  container_not_running: 'muted',
  skipped_concurrency: 'muted',
  replaced: 'muted',
}

// JobStatusBadge shows a run status: green for a healthy outcome, red or
// amber for something an operator should look at, muted for expected
// policy side effects that are not failures.
export function JobStatusBadge({ status }: { status: ScheduledTaskRunStatus }) {
  const { t } = useTranslation('jobs')
  return <Badge variant={VARIANT[status]}>{t(`status.${status}`)}</Badge>
}
