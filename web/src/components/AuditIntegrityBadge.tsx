import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ShieldCheckIcon,
  ShieldWarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { auditChainQueryOptions } from '../queries/auditLog'

export function AuditIntegrityBadge() {
  const { t } = useTranslation('auditLog')
  const { data, isPending, isFetching, isError, refetch } = useQuery(
    auditChainQueryOptions(),
  )

  let badge = (
    <Badge variant="muted">
      {isPending ? t('integrity.checking') : t('integrity.unavailable')}
    </Badge>
  )
  if (data?.ok) {
    badge = (
      <Badge
        variant="success"
        title={t('integrity.intactDetail', {
          checked: data.checked,
          checkpoints: data.checkpoints,
        })}
      >
        <ShieldCheckIcon aria-hidden="true" />
        {t('integrity.intact')}
      </Badge>
    )
  } else if (data) {
    badge = (
      <Badge
        variant="destructive"
        title={data.problems.map((p) => p.reason).join('\n')}
      >
        <ShieldWarningIcon aria-hidden="true" />
        {t('integrity.failed')}
      </Badge>
    )
  }

  return (
    <span className="inline-flex items-center gap-1.5">
      {badge}
      {data && !data.ok ? (
        <span className="sr-only">
          {t('integrity.failedDetail', { count: data.problems.length })}
        </span>
      ) : null}
      {data || isError ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={isFetching}
          onClick={() => {
            void refetch()
          }}
        >
          {t('integrity.recheck')}
        </Button>
      ) : null}
    </span>
  )
}
