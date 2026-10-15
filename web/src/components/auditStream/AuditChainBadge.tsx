import { useTranslation } from 'react-i18next'
import {
  ArrowClockwiseIcon,
  ShieldCheckIcon,
  ShieldWarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useAuditChain } from '../../queries/auditStream'

const KNOWN_REASONS = [
  'entry_hash_mismatch',
  'prev_hash_mismatch',
  'seq_gap',
  'tail_truncated',
] as const

export function AuditChainBadge() {
  const { t } = useTranslation('auditStream')
  const { data, isPending, isFetching, isError, refetch } = useAuditChain()

  let badge: React.ReactNode
  let detail = ''
  if (isPending) {
    badge = <Badge variant="muted">{t('chain.checking')}</Badge>
  } else if (isError) {
    badge = <Badge variant="muted">{t('chain.unavailable')}</Badge>
  } else if (data.ok) {
    detail = t('chain.intactDetail', {
      chained: data.chained,
      unchained: data.unchained,
    })
    badge = (
      <Badge variant="success">
        <ShieldCheckIcon className="size-3.5" aria-hidden="true" />
        {t('chain.intact')}
      </Badge>
    )
  } else {
    const broken = data.first_broken
    const known = KNOWN_REASONS.find((r) => r === broken?.reason)
    detail = t('chain.brokenDetail', {
      seq: broken?.seq ?? 0,
      id: broken?.id ? ` (${broken.id})` : '',
      reason: known ? t(`chain.reasons.${known}`) : (broken?.reason ?? ''),
    })
    badge = (
      <Badge variant="destructive">
        <ShieldWarningIcon className="size-3.5" aria-hidden="true" />
        {t('chain.broken', { seq: broken?.seq ?? 0 })}
      </Badge>
    )
  }

  return (
    <div className="flex flex-wrap items-center gap-2" title={detail}>
      {badge}
      {detail ? (
        <span className="text-xs text-muted-foreground">{detail}</span>
      ) : null}
      <Button
        variant="ghost"
        size="sm"
        disabled={isFetching}
        aria-label={t('chain.recheck')}
        onClick={() => {
          void refetch()
        }}
      >
        <ArrowClockwiseIcon className="size-3.5" aria-hidden="true" />
      </Button>
    </div>
  )
}
