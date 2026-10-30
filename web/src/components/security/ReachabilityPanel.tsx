import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  GlobeGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import {
  type ReachFinding,
  reachabilityQueryOptions,
  useApplyReachFix,
  useRecheckReachability,
} from '../../queries/reachability'

const SEVERITY_VARIANT = {
  high: 'destructive',
  medium: 'warning',
  low: 'muted',
  info: 'muted',
} as const

function FindingRow({
  finding,
  busy,
  onFix,
}: {
  finding: ReachFinding
  busy: boolean
  onFix: (f: ReachFinding) => void
}) {
  const { t } = useTranslation('security')
  const router = useRouter()
  const fix = finding.fix
  const subject = finding.subject.name ?? ''
  return (
    <li
      className="flex flex-wrap items-start gap-3 py-3"
      data-testid={`reach-${finding.id}`}
    >
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium">
            {t(`reach.kinds.${finding.kind}.title`, {
              name: subject,
              ...finding.params,
            })}
          </p>
          <Badge variant={SEVERITY_VARIANT[finding.severity]}>
            {t(`reach.severity.${finding.severity}`)}
          </Badge>
        </div>
        <p className="text-sm text-muted-foreground">
          {t(`reach.kinds.${finding.kind}.why`)}
        </p>
      </div>
      {fix?.kind === 'auto' ? (
        <Button
          type="button"
          size="sm"
          disabled={busy}
          onClick={() => onFix(finding)}
        >
          {t(`reach.kinds.${finding.kind}.fix`)}
        </Button>
      ) : null}
      {fix?.kind === 'guided' && fix.link ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            router.history.push(fix.link ?? '/')
          }}
        >
          {t(`reach.kinds.${finding.kind}.fix`)}
        </Button>
      ) : null}
    </li>
  )
}

export function ReachabilityPanel() {
  const { t } = useTranslation('security')
  const report = useQuery(reachabilityQueryOptions())
  const recheck = useRecheckReachability()
  const apply = useApplyReachFix()
  const [confirm, setConfirm] = useState<ReachFinding | null>(null)

  function run(finding: ReachFinding) {
    if (!finding.fix) return
    apply.mutate(finding.fix, {
      onSuccess: (rep) => {
        const still = rep.findings.some((f) => f.id === finding.id)
        toast.add({
          title: still ? t('reach.stillFailing') : t('reach.fixed'),
          type: still ? 'error' : 'success',
        })
        setConfirm(null)
      },
      onError: (e) =>
        toast.add({
          title: e.message || t('reach.applyError'),
          type: 'error',
        }),
    })
  }

  function onFix(finding: ReachFinding) {
    if (finding.fix?.restarts) setConfirm(finding)
    else run(finding)
  }

  const rep = report.data
  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            <GlobeGlassIcon className="size-5 text-muted-foreground" />
            {t('reach.title')}
          </CardTitle>
          <CardDescription>{t('reach.description')}</CardDescription>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={recheck.isPending}
          onClick={() =>
            recheck.mutate(undefined, {
              onError: (e) =>
                toast.add({
                  title: e.message || t('reach.recheckError'),
                  type: 'error',
                }),
            })
          }
        >
          <RefreshCwGlassIcon className="size-5" />
          {recheck.isPending ? t('reach.rechecking') : t('reach.recheck')}
        </Button>
      </CardHeader>
      <CardContent className="space-y-6">
        {report.error ? (
          <p className="text-sm text-destructive">{t('reach.loadError')}</p>
        ) : null}
        {rep && !rep.rules_readable ? (
          <p className="text-sm text-muted-foreground">
            {t('reach.rulesUnreadable')}
          </p>
        ) : null}
        {rep ? (
          rep.findings.length === 0 ? (
            <p className="flex items-center gap-2 text-sm">
              <CircleCheckGlassIcon className="size-5 text-green-700 dark:text-green-400" />
              {t('reach.clean', { count: rep.restricted_ports })}
            </p>
          ) : (
            <ul className="divide-y">
              {rep.findings.map((f) => (
                <FindingRow
                  key={f.id}
                  finding={f}
                  busy={apply.isPending}
                  onFix={onFix}
                />
              ))}
            </ul>
          )
        ) : null}
        {rep && rep.subjects.length > 0 ? (
          <div className="space-y-2">
            <h3 className="text-sm font-medium">{t('reach.subjects')}</h3>
            <ul className="divide-y text-sm">
              {rep.subjects.map((s) => (
                <li
                  key={`${s.type}:${s.name}`}
                  className="flex flex-wrap gap-x-3 gap-y-1 py-2"
                >
                  <span className="font-medium">{s.name}</span>
                  <Badge variant="outline">{t(`reach.type.${s.type}`)}</Badge>
                  <span className="min-w-0 break-all text-muted-foreground">
                    {s.endpoints.length === 0
                      ? t('reach.private')
                      : s.endpoints.map((e) => e.value).join(', ')}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
      <ConfirmDialog
        open={confirm !== null}
        onOpenChange={(o) => {
          if (!o) setConfirm(null)
        }}
        title={t('reach.confirm.title')}
        description={t('reach.confirm.description')}
        confirmLabel={t('reach.confirm.confirm')}
        pendingLabel={t('reach.confirm.pending')}
        pending={apply.isPending}
        onConfirm={() => {
          if (confirm) run(confirm)
        }}
      />
    </Card>
  )
}
