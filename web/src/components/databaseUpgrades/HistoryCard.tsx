import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import type { UpgradeRun } from '../../types/databaseUpgrades'
import { RUN_STEPS, kindVariant, stateVariant } from './upgradeHelpers'

function formatWhen(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

function kindBadgeVariant(kind: string) {
  return kind === 'patch' || kind === 'minor' || kind === 'major'
    ? kindVariant(kind)
    : 'muted'
}

function ActiveRun({ run }: { run: UpgradeRun }) {
  const { t } = useTranslation('databaseUpgrades')
  const index = RUN_STEPS.indexOf(run.state)
  return (
    <div className="space-y-2 rounded-lg border p-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <span className="font-medium">{t('history.active')}</span>
        <span className="font-mono">
          {t('history.from', { from: run.from_version, to: run.to_version })}
        </span>
      </div>
      <ol className="flex flex-wrap gap-1.5">
        {RUN_STEPS.map((step, i) => (
          <li key={step}>
            <Badge
              variant={
                i < index ? 'success' : i === index ? 'default' : 'muted'
              }
              aria-current={i === index ? 'step' : undefined}
            >
              {t(`history.state.${step}`)}
            </Badge>
          </li>
        ))}
      </ol>
      {run.phase ? (
        <p className="text-xs text-muted-foreground">{run.phase}</p>
      ) : null}
    </div>
  )
}

function RunRow({ run }: { run: UpgradeRun }) {
  const { t } = useTranslation('databaseUpgrades')
  return (
    <li className="space-y-1 py-3 text-sm first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono">
          {t('history.from', { from: run.from_version, to: run.to_version })}
        </span>
        <Badge variant={kindBadgeVariant(run.kind)}>{run.kind}</Badge>
        <Badge variant="outline">{t(`history.source.${run.source}`)}</Badge>
        <Badge variant={stateVariant(run.state)}>
          {t(`history.state.${run.state}`)}
        </Badge>
        {run.revert_path ? (
          <span className="text-muted-foreground">
            {t(`history.revert.${run.revert_path}`)}
          </span>
        ) : null}
      </div>
      {run.reason ? (
        <p className="text-muted-foreground">
          {t('history.reason', { reason: run.reason })}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-muted-foreground">
        {run.backup_id ? (
          <span className="font-mono">
            {t('history.backup', { id: run.backup_id })}
          </span>
        ) : null}
        <span>
          {t('history.started', { date: formatWhen(run.created_at) })}
        </span>
        {run.finished_at ? (
          <span>
            {t('history.finished', { date: formatWhen(run.finished_at) })}
          </span>
        ) : null}
      </div>
    </li>
  )
}

/** HistoryCard shows the in-flight upgrade's progress above the capped list of past runs. */
export function HistoryCard({
  active,
  history,
}: {
  active?: UpgradeRun
  history: UpgradeRun[]
}) {
  const { t } = useTranslation('databaseUpgrades')
  const past = history.filter((r) => r.id !== active?.id)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('history.title')}</CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {active ? <ActiveRun run={active} /> : null}
        {past.length === 0 && !active ? (
          <p className="text-sm text-muted-foreground">{t('history.empty')}</p>
        ) : (
          <ul className="divide-y">
            {past.map((run) => (
              <RunRow key={run.id} run={run} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
