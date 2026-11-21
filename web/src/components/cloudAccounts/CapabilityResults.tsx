import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/ui/status-badge'
import type { CloudCapabilityResult } from '../../types/cloudAccounts'

export function CapabilityResults({
  results,
}: Readonly<{ results: readonly CloudCapabilityResult[] }>) {
  const { t } = useTranslation('cloudAccounts')
  return (
    <ul className="divide-y divide-border rounded-lg border border-border">
      {results.map((r) => (
        <li
          key={r.name}
          className="flex flex-wrap items-start gap-x-3 gap-y-1 px-3 py-2"
        >
          <span className="w-24 shrink-0 text-sm font-medium">
            {t(`capabilities.${r.name}.name`, { defaultValue: r.name })}
          </span>
          <StatusBadge
            state={r.ok ? 'healthy' : 'failed'}
            label={r.ok ? t('probe.ok') : t('probe.missing')}
          />
          <span className="text-xs text-muted-foreground">
            {r.verified ? t('probe.verified') : t('probe.simulated')}
          </span>
          {!r.ok ? (
            <div className="basis-full space-y-0.5 text-xs">
              {r.hint ? <p className="text-foreground">{r.hint}</p> : null}
              {r.missing?.length ? (
                <p className="font-mono break-all text-muted-foreground">
                  {r.missing.join(', ')}
                </p>
              ) : null}
            </div>
          ) : null}
        </li>
      ))}
    </ul>
  )
}
