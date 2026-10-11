import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { AppImportCutoverGuide } from './AppImportCutoverGuide'
import {
  appImportReceiptUrl,
  useAppImportCutover,
  useRouteAppImport,
  type AppImportCutoverDomain,
  type AppImportCutoverItem,
  type AppImportSession,
} from '../queries/appImport'

const verdictVariant: Record<
  AppImportCutoverDomain['verdict'],
  'success' | 'warning' | 'destructive'
> = {
  go: 'success',
  switched: 'success',
  wait: 'warning',
  'no-go': 'destructive',
}

const checkVariant = {
  pass: 'success',
  warn: 'warning',
  fail: 'destructive',
} as const

function DomainRow({ d }: { d: AppImportCutoverDomain }) {
  const { t } = useTranslation('migration')
  const showChange = d.change && (d.verdict === 'go' || d.verdict === 'wait')
  return (
    <li className="rounded-md border p-2 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{d.domain}</span>
        <Badge variant={verdictVariant[d.verdict]}>
          {t(`cutover.verdict.${d.verdict}`)}
        </Badge>
      </div>
      <ul className="mt-2 space-y-1">
        {d.checks.map((c) => (
          <li key={c.id} className="flex flex-wrap items-start gap-2">
            <Badge variant={checkVariant[c.status]}>{c.status}</Badge>
            <span className="flex-1">
              {c.detail}
              {c.fix ? (
                <span className="block text-muted-foreground">{c.fix}</span>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
      {showChange && d.change ? (
        <div className="mt-2 rounded-md bg-muted p-2 font-mono text-xs">
          <p>
            {t('cutover.change', {
              type: d.change.type,
              name: d.change.name,
              value: d.change.value,
              ttl: d.change.ttl,
            })}
          </p>
          {d.change.replaces ? (
            <p className="text-muted-foreground">
              {t('cutover.replaces', { what: d.change.replaces })}
            </p>
          ) : null}
        </div>
      ) : null}
    </li>
  )
}

function ItemCard({
  item,
  sessionId,
  disabled,
}: {
  item: AppImportCutoverItem
  sessionId: string
  disabled: boolean
}) {
  const { t } = useTranslation('migration')
  const route = useRouteAppImport()
  const [force, setForce] = useState(false)
  return (
    <li className="space-y-2 rounded-md border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{item.name}</span>
        <Badge variant={item.routed ? 'success' : 'default'}>
          {item.routed
            ? t('appImport.cutover.routed')
            : t('appImport.cutover.notRouted')}
        </Badge>
        {item.volumes_pending > 0 ? (
          <Badge variant="warning">
            {t('appImport.cutover.volumesPending', {
              count: item.volumes_pending,
            })}
          </Badge>
        ) : null}
        <div className="ml-auto flex items-center gap-2">
          {item.routed ? (
            <Button
              size="sm"
              variant="outline"
              disabled={disabled || route.isPending}
              onClick={() =>
                route.mutate({
                  id: sessionId,
                  item: item.source_id,
                  enable: false,
                })
              }
            >
              {t('appImport.cutover.disableRouting')}
            </Button>
          ) : (
            <Button
              size="sm"
              disabled={disabled || route.isPending}
              onClick={() =>
                route.mutate(
                  {
                    id: sessionId,
                    item: item.source_id,
                    enable: true,
                    ignoreVolumes: force,
                  },
                  {
                    onError: (e) => {
                      toast.add({ title: e.message, type: 'error' })
                      if (item.volumes_pending > 0) setForce(true)
                    },
                  },
                )
              }
            >
              {force
                ? t('appImport.cutover.enableAnyway')
                : t('appImport.cutover.enableRouting')}
            </Button>
          )}
        </div>
      </div>
      {route.isError ? (
        <p className="text-xs text-destructive">{route.error.message}</p>
      ) : null}
      <ul className="space-y-2">
        {item.domains.map((d) => (
          <DomainRow key={d.domain} d={d} />
        ))}
      </ul>
      <AppImportCutoverGuide
        sessionId={sessionId}
        item={item.source_id}
        app={item.target}
      />
    </li>
  )
}

export function AppImportCutover({
  session,
  onBack,
}: {
  session: AppImportSession
  onBack: () => void
}) {
  const { t } = useTranslation('migration')
  const [targetIp, setTargetIp] = useState('')
  const check = useAppImportCutover()
  const report = check.data

  function run(verify: boolean) {
    check.mutate({
      id: session.id,
      verify,
      targetIp: targetIp.trim() || undefined,
    })
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {t('appImport.cutover.title')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('appImport.cutover.description')}
        </p>
      </div>
      <div className="rounded-md border bg-muted/30 p-3 text-sm">
        <p className="font-medium">{t('appImport.cutover.keepSource')}</p>
        <p className="text-muted-foreground">
          {t('appImport.cutover.keepSourceBody')}
        </p>
      </div>
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-48 space-y-1.5">
          <Label htmlFor="appimport-cutover-ip">{t('cutover.targetIp')}</Label>
          <Input
            id="appimport-cutover-ip"
            placeholder="203.0.113.10"
            value={targetIp}
            onChange={(e) => setTargetIp(e.target.value)}
          />
        </div>
        <Button disabled={check.isPending} onClick={() => run(false)}>
          {check.isPending ? t('cutover.running') : t('cutover.check')}
        </Button>
        <Button
          variant="outline"
          disabled={check.isPending}
          onClick={() => run(true)}
        >
          {t('cutover.verify')}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('cutover.targetIpHelp')}
      </p>
      {check.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{check.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {report ? (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={verdictVariant[report.verdict]}>
              {t(`cutover.verdict.${report.verdict}`)}
            </Badge>
            <span className="text-muted-foreground">
              {t(`cutover.phase.${report.phase}`)}
              {report.target_ips.length > 0
                ? `. ${t('cutover.thisNode', { ips: report.target_ips.join(', ') })}`
                : ''}
            </span>
          </div>
          {report.items.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('appImport.cutover.none')}
            </p>
          ) : (
            <ul className="space-y-3">
              {report.items.map((it) => (
                <ItemCard
                  key={it.source_id}
                  item={it}
                  sessionId={session.id}
                  disabled={session.running}
                />
              ))}
            </ul>
          )}
          <ul className="list-disc space-y-1 pl-5 text-xs text-muted-foreground">
            {report.guidance.map((g) => (
              <li key={g}>{g}</li>
            ))}
          </ul>
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2 border-t pt-3">
        <Button variant="outline" onClick={onBack}>
          {t('appImport.back')}
        </Button>
        <Button
          variant="outline"
          render={
            <a href={appImportReceiptUrl(session.id)} download>
              {t('appImport.cutover.receipt')}
            </a>
          }
        />
        <span className="text-xs text-muted-foreground">
          {t('appImport.cutover.receiptNote')}
        </span>
      </div>
    </div>
  )
}
