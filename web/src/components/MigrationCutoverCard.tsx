import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useCutoverCheck,
  type CutoverDomain,
  type Verdict,
} from '../queries/migration'

const verdictVariant: Record<
  Verdict,
  'success' | 'warning' | 'destructive' | 'default'
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

function DomainRow({ d }: { d: CutoverDomain }) {
  const { t } = useTranslation('migration')
  const showChange = d.change && (d.verdict === 'go' || d.verdict === 'wait')
  return (
    <li className="rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{d.domain}</span>
        <span className="text-xs text-muted-foreground">{d.app}</span>
        <Badge variant={verdictVariant[d.verdict]}>
          {t(`cutover.verdict.${d.verdict}`)}
        </Badge>
      </div>
      <ul className="mt-2 space-y-1.5">
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

export function MigrationCutoverCard() {
  const { t } = useTranslation('migration')
  const [targetIp, setTargetIp] = useState('')
  const check = useCutoverCheck()
  const report = check.data

  function run(verify: boolean) {
    check.mutate({ verify, targetIp: targetIp.trim() || undefined })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('cutover.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('cutover.description')}
        </p>
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-48 space-y-1.5">
            <Label htmlFor="cutover-ip">{t('cutover.targetIp')}</Label>
            <Input
              id="cutover-ip"
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
            <AlertDescription>
              {t('cutover.loadError')}: {check.error.message}
            </AlertDescription>
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
            {report.domains.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('cutover.noDomains')}
              </p>
            ) : (
              <ul className="space-y-2">
                {report.domains.map((d) => (
                  <DomainRow key={`${d.app}-${d.domain}`} d={d} />
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
      </CardContent>
    </Card>
  )
}
