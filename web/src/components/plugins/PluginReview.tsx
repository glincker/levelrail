import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  ShieldAlertGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { StatusBadge } from '@/components/ui/status-badge'
import type { PluginPreview, PluginVerification } from '../../queries/plugins'

export function VerificationBadge({
  verification,
}: Readonly<{ verification: Pick<PluginVerification, 'status'> }>) {
  const { t } = useTranslation('plugins')
  const label = t(`verification.${verification.status}`)
  if (verification.status === 'verified') {
    return (
      <StatusBadge tone="success" icon={CircleCheckGlassIcon} label={label} />
    )
  }
  if (verification.status === 'unsigned') {
    return (
      <StatusBadge tone="warning" icon={TriangleAlertGlassIcon} label={label} />
    )
  }
  return <StatusBadge tone="danger" icon={ShieldAlertGlassIcon} label={label} />
}

function AbilityRow({
  ability,
  added,
}: Readonly<{ ability: string; added: boolean }>) {
  const { t } = useTranslation('plugins')
  const key = ability === 'read:sensitive' ? 'readSensitive' : 'read'
  return (
    <li className="flex items-start gap-2 text-sm">
      <code className="rounded bg-muted px-1 py-0.5 text-xs">{ability}</code>
      <span className="text-muted-foreground">{t(`abilities.${key}`)}</span>
      {added ? (
        <StatusBadge
          tone="warning"
          icon={TriangleAlertGlassIcon}
          label={t('review.newAbility')}
        />
      ) : null}
    </li>
  )
}

export function PluginReview({
  preview,
}: Readonly<{ preview: PluginPreview }>) {
  const { t } = useTranslation('plugins')
  const { manifest: m, verification: v } = preview
  const upgrade = preview.action === 'upgrade'
  return (
    <div className="space-y-4">
      <div>
        <p className="font-medium">
          {m.name} {m.version}
        </p>
        <p className="text-sm text-muted-foreground">
          {t('publisher', { publisher: m.publisher })}
        </p>
        {m.description ? <p className="mt-1 text-sm">{m.description}</p> : null}
        {upgrade ? (
          <p className="mt-1 text-xs text-muted-foreground">
            {t('review.upgradeFrom', { version: preview.installed_version })}
          </p>
        ) : null}
      </div>
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium">
            {t('review.verification')}
          </span>
          <VerificationBadge verification={v} />
        </div>
        <p className="text-xs text-muted-foreground">
          {t('review.digest', { digest: v.digest })}
        </p>
        {v.signed ? (
          <p className="text-xs text-muted-foreground">
            {t('review.signedBy', { key: v.key_name || v.key_id })}
          </p>
        ) : null}
        {v.installable ? null : (
          <Alert variant="destructive">
            <AlertTitle>{t('review.refused')}</AlertTitle>
            <AlertDescription>{v.reason}</AlertDescription>
          </Alert>
        )}
        {v.status === 'unsigned' ? (
          <p className="text-xs text-destructive">{t('review.unsignedWarn')}</p>
        ) : null}
      </div>
      <div className="space-y-2">
        <p className="text-sm font-medium">{t('review.abilities')}</p>
        <ul className="space-y-1">
          {m.abilities.map((a) => (
            <AbilityRow
              key={a}
              ability={a}
              added={upgrade && preview.added_abilities.includes(a)}
            />
          ))}
        </ul>
        {preview.removed_abilities.length > 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('review.removed', {
              abilities: preview.removed_abilities.join(', '),
            })}
          </p>
        ) : null}
      </div>
      <div className="space-y-1">
        <p className="text-sm font-medium">{t('approve.network')}</p>
        <p className="text-sm text-muted-foreground">
          {m.outbound_hosts.length === 0
            ? t('approve.networkNone')
            : t('approve.networkHosts', { hosts: m.outbound_hosts.join(', ') })}
        </p>
        {m.outbound_hosts.length > 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('approve.networkNote')}
          </p>
        ) : null}
      </div>
      {m.contributes.panels.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('review.panels', { count: m.contributes.panels.length })}
        </p>
      ) : null}
      {preview.reapproval_required ? (
        <Alert>
          <AlertTitle>{t('review.reapproval')}</AlertTitle>
          <AlertDescription>{t('review.reapprovalHint')}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
