import { useTranslation } from 'react-i18next'
import { BroomIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useOrphans, useReapOrphans } from '../queries/orphans'
import type { OrphanFinding } from '../queries/orphans'

const MAX_ROWS = 50

function statusKey(
  f: OrphanFinding,
): 'orphans.status.kept' | 'orphans.status.due' | 'orphans.status.grace' {
  if (f.skip) return 'orphans.status.kept'
  if (f.due) return 'orphans.status.due'
  return 'orphans.status.grace'
}

export function OrphanReaperCard() {
  const { t } = useTranslation('settings')
  const { data: report, isLoading } = useOrphans()
  const reap = useReapOrphans()

  if (isLoading || !report) return null
  const shown = report.findings.slice(0, MAX_ROWS)
  const dueCount = report.findings.filter((f) => f.due).length

  function run(dryRun: boolean) {
    reap.mutate(dryRun, {
      onSuccess: (r) => {
        toast.add({
          title: dryRun
            ? t('orphans.toast.dryRun', {
                count: r.findings.filter((f) => f.due).length,
              })
            : t('orphans.toast.reaped', { count: r.removed.length }),
          type: r.failed && r.failed.length > 0 ? 'error' : 'success',
        })
      },
      onError: (e) => toast.add({ title: e.message, type: 'error' }),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-1.5">
          <BroomIcon className="size-4" aria-hidden="true" />
          {t('orphans.title')}
        </CardTitle>
        <CardDescription>{t('orphans.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {report.findings.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('orphans.empty')}</p>
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {shown.map((f) => (
              <li
                key={`${f.kind}:${f.node_id}:${f.name}`}
                className="flex flex-wrap items-center gap-2 px-3 py-2 text-sm"
              >
                <Badge variant="outline">{t(`orphans.kind.${f.kind}`)}</Badge>
                <span className="font-medium text-foreground">{f.name}</span>
                <span className="text-muted-foreground">
                  {f.node_id || t('orphans.localNode')}
                </span>
                <span className="ml-auto text-muted-foreground">
                  {t(`orphans.reason.${f.reason}`, { defaultValue: f.reason })}
                </span>
                <Badge variant={f.due ? 'warning' : 'muted'}>
                  {t(statusKey(f))}
                </Badge>
                {f.skip ? (
                  <span className="basis-full text-xs text-muted-foreground">
                    {f.skip}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        )}
        {report.findings.length > MAX_ROWS ? (
          <p className="text-xs text-muted-foreground">
            {t('orphans.more', { count: report.findings.length - MAX_ROWS })}
          </p>
        ) : null}
        {report.halted ? (
          <p className="text-xs text-muted-foreground">{report.halted}</p>
        ) : null}
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={reap.isPending}
            onClick={() => run(true)}
          >
            {t('orphans.dryRun')}
          </Button>
          <Button
            size="sm"
            disabled={reap.isPending || dueCount === 0}
            onClick={() => run(false)}
          >
            {t('orphans.reapNow', { count: dueCount })}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
