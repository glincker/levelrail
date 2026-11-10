import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import type { PlatformImportReport } from '../queries/platformImport'
import type { ExtrasToggles } from '../lib/platformImportExtras'

function endpointHost(endpoint?: string): string {
  if (!endpoint) return ''
  try {
    return new URL(endpoint).host
  } catch {
    return ''
  }
}

function stateVariant(
  state: string,
): 'success' | 'warning' | 'destructive' | 'muted' {
  if (state === 'created' || state === 'would_create') return 'success'
  if (state === 'failed') return 'destructive'
  if (state === 'exists' || state === 'skipped') return 'muted'
  return 'warning'
}

const stateKeys = {
  would_create: 'platformImport.extras.state.would_create',
  needs_secret: 'platformImport.extras.state.needs_secret',
  created: 'platformImport.extras.state.created',
  exists: 'platformImport.extras.state.exists',
  pending: 'platformImport.extras.state.pending',
  skipped: 'platformImport.extras.state.skipped',
  review: 'platformImport.extras.state.review',
  failed: 'platformImport.extras.state.failed',
  unsupported: 'platformImport.extras.state.unsupported',
} as const

function stateLabel(t: TFunction<'migration'>, state: string): string {
  const key = stateKeys[state as keyof typeof stateKeys]
  return key ? t(key) : state
}

function Row({
  title,
  meta,
  state,
  detail,
}: {
  title: string
  meta: string[]
  state: string
  detail?: string
}) {
  const { t } = useTranslation('migration')
  return (
    <li className="space-y-1 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-foreground">{title}</span>
        {meta
          .filter((m) => m !== '')
          .map((m) => (
            <Badge key={m} variant="outline">
              {m}
            </Badge>
          ))}
        <Badge variant={stateVariant(state)}>{stateLabel(t, state)}</Badge>
      </div>
      {detail ? (
        <p className="break-words font-mono text-xs text-muted-foreground">
          {detail}
        </p>
      ) : null}
    </li>
  )
}

// Backup targets, schedules and alert channels from the source. The API
// returns names, shapes and has_* flags only, never a key or webhook URL.
export function PlatformImportExtras({
  report,
}: {
  report: PlatformImportReport
}) {
  const { t } = useTranslation('migration')
  const targets = report.backups?.targets ?? []
  const schedules = report.backups?.schedules ?? []
  const channels = report.channels?.channels ?? []
  const unsupported = report.channels?.unsupported ?? []
  return (
    <section
      className="space-y-2"
      aria-label={t('platformImport.extras.title')}
    >
      <h3 className="text-sm font-medium text-foreground">
        {t('platformImport.extras.title')}
      </h3>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {targets.map((x) => (
          <Row
            key={`t-${x.name}-${x.source_name}`}
            title={t('platformImport.extras.target', {
              name: x.name || x.source_name,
            })}
            meta={[
              x.provider,
              endpointHost(x.endpoint),
              x.bucket,
              x.has_keys
                ? t('platformImport.extras.hasKeys')
                : t('platformImport.extras.noKeys'),
            ]}
            state={x.state}
            detail={x.detail}
          />
        ))}
        {schedules.map((x, i) => (
          <Row
            key={`s-${x.database}-${i}`}
            title={t('platformImport.extras.schedule', { name: x.database })}
            meta={[
              x.cron,
              x.retain
                ? t('platformImport.extras.retain', { count: x.retain })
                : '',
              x.paused
                ? t('platformImport.extras.paused')
                : t('platformImport.extras.running'),
            ]}
            state={x.state}
            detail={x.detail}
          />
        ))}
        {channels.map((x) => (
          <Row
            key={`c-${x.name}-${x.source_name}`}
            title={t('platformImport.extras.channel', {
              name: x.name || x.source_name,
            })}
            meta={[
              x.kind,
              x.has_secret
                ? t('platformImport.extras.hasSecret')
                : t('platformImport.extras.noSecret'),
              x.enabled
                ? t('platformImport.extras.enabled')
                : t('platformImport.extras.disabled'),
            ]}
            state={x.state}
            detail={x.detail}
          />
        ))}
        {unsupported.map((x) => (
          <Row
            key={`u-${x.source_name}-${x.source_kind}`}
            title={t('platformImport.extras.channel', { name: x.source_name })}
            meta={[x.source_kind]}
            state="unsupported"
            detail={x.reason}
          />
        ))}
      </ul>
    </section>
  )
}

const toggleKeys = [
  {
    key: 'includeCredentials',
    label: 'platformImport.extras.includeCredentials',
    warning: 'platformImport.extras.includeCredentialsWarning',
  },
  {
    key: 'enableSchedules',
    label: 'platformImport.extras.enableSchedules',
    warning: 'platformImport.extras.enableSchedulesWarning',
  },
  {
    key: 'enableChannels',
    label: 'platformImport.extras.enableChannels',
    warning: 'platformImport.extras.enableChannelsWarning',
  },
] as const

export function PlatformImportExtrasToggles({
  value,
  onChange,
}: {
  value: ExtrasToggles
  onChange: (next: ExtrasToggles) => void
}) {
  const { t } = useTranslation('migration')
  return (
    <div className="space-y-2">
      {toggleKeys.map(({ key, label, warning }) => (
        <Label key={key} className="flex items-start gap-2 font-normal">
          <Checkbox
            aria-label={t(label)}
            checked={value[key]}
            onCheckedChange={(v) => onChange({ ...value, [key]: v === true })}
            className="mt-0.5"
          />
          <span className="space-y-0.5">
            <span className="block text-foreground">{t(label)}</span>
            <span className="block text-xs text-muted-foreground">
              {t(warning)}
            </span>
          </span>
        </Label>
      ))}
    </div>
  )
}
