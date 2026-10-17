import { useTranslation } from 'react-i18next'
import { Progress } from '@/components/ui/progress'
import { formatDate } from '../../lib/format'
import type { NodeResource } from '../../types/nodeDetail'
import type { NodeTrustResource } from '../../types/nodeTrust'
import { Fingerprint } from './Fingerprint'
import { TrustItem } from './TrustItem'

const PERCENT = 100

// The facts behind the status line: which CA this node was pinned to, the
// certificate it presents, and when it renews.
export function NodeTrustDetails({
  node,
  trust,
}: {
  node: NodeResource
  trust: NodeTrustResource
}) {
  const { t } = useTranslation('nodes')
  const never = t('trust.never')
  const { ca, certificate: cert, renewal, validation } = trust
  const used = cert?.lifetime_used
  return (
    <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <TrustItem label={t('trust.caFingerprint')} wide>
        <Fingerprint value={ca?.fingerprint} />
        <span className="mt-1 block text-xs text-muted-foreground">
          {t('trust.caCompare')}
        </span>
        <span className="mt-1 block text-xs text-muted-foreground">
          {t(`trust.pin.${ca?.pin_state ?? 'unknown'}`)}
        </span>
      </TrustItem>
      <TrustItem label={t('trust.certFingerprint')} wide>
        <Fingerprint value={cert?.fingerprint} />
      </TrustItem>
      <TrustItem label={t('trust.serial')}>
        <code className="font-mono text-xs break-all">
          {cert?.serial || t('trust.unavailable')}
        </code>
      </TrustItem>
      <TrustItem label={t('trust.issued')}>
        {formatDate(cert?.issued_at, t('trust.unavailable'))}
      </TrustItem>
      <TrustItem label={t('trust.expires')} wide>
        {formatDate(cert?.not_after, t('trust.unavailable'))}
        {cert?.days_remaining !== undefined
          ? ` (${t('trust.daysRemaining', { count: cert.days_remaining })})`
          : ''}
        {used !== undefined ? (
          <Progress
            value={Math.round(used * PERCENT)}
            aria-label={t('trust.lifetimeUsed')}
            className="mt-2"
          />
        ) : null}
      </TrustItem>
      <TrustItem label={t('trust.nextRenewal')}>
        {formatDate(renewal?.next_renew_at, t('trust.unavailable'))}
        {renewal?.next_renew_estimated ? ` (${t('trust.estimated')})` : ''}
      </TrustItem>
      <TrustItem label={t('trust.lastRenewed')}>
        {renewal?.last_renewed_at
          ? formatDate(renewal.last_renewed_at, never)
          : t('trust.notSinceEnrollment')}
        {` (${t('trust.renewalCount', { count: renewal?.renewal_count ?? 0 })})`}
      </TrustItem>
      {renewal?.last_error ? (
        <TrustItem label={t('trust.lastFailure')} wide>
          <span className="text-tone-danger">{renewal.last_error}</span>
          <span className="mt-1 block text-xs text-muted-foreground">
            {formatDate(renewal.last_failed_at, never)}
            {renewal.next_retry_at
              ? `, ${t('trust.nextRetry', {
                  at: formatDate(renewal.next_retry_at, never),
                })}`
              : ''}
          </span>
        </TrustItem>
      ) : null}
      <TrustItem label={t('trust.privateKey')} wide>
        {t(
          `trust.key.${cert?.key_origin === 'reenroll_required' ? 'reenroll_required' : 'agent'}`,
        )}
      </TrustItem>
      {validation ? (
        <TrustItem
          label={t('trust.validation', { hours: validation.window_hours })}
        >
          {t('trust.validationCounts', {
            accepted: validation.accepted,
            rejected: validation.rejected,
          })}
        </TrustItem>
      ) : null}
      <TrustItem label={t('agent.version')}>
        {node.agent?.version || t('agent.notReported')}
        {node.agent?.min_version
          ? ` (${t('agent.minimum', { version: node.agent.min_version })})`
          : ''}
      </TrustItem>
      <TrustItem label={t('agent.platform')}>
        {node.agent?.os
          ? `${node.agent.os}/${node.agent.arch ?? ''}`
          : t('agent.notReported')}
      </TrustItem>
    </dl>
  )
}
