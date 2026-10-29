import { useTranslation } from 'react-i18next'
import { PinGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { RelativeTime } from '@/components/kit/RelativeTime'
import { useRollbackTargets } from '../queries/deployGate'

/** RollbackTargetsList shows the releases pinned so image cleanup can never remove them. */
export function RollbackTargetsList({ appName }: { appName: string }) {
  const { t } = useTranslation('deploySafety')
  const { data: targets } = useRollbackTargets(appName)

  return (
    <section className="space-y-2" aria-label={t('targets.title')}>
      <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {t('targets.title')}
      </h3>
      {!targets || targets.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('targets.empty')}</p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {targets.map((target) => (
            <li
              key={target.deploy_id}
              className="flex flex-wrap items-center gap-2 px-3 py-2 text-sm"
            >
              <PinGlassIcon
                className="size-4 text-muted-foreground"
                aria-hidden="true"
              />
              <span className="font-mono text-xs break-all">
                {target.image}
              </span>
              {target.live ? (
                <Badge variant="success">{t('targets.live')}</Badge>
              ) : null}
              <span className="ml-auto text-xs text-muted-foreground">
                {target.deploy_id} &middot;{' '}
                <RelativeTime at={target.pinned_at} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
