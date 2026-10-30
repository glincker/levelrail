import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useProbeDomain, type ProbeCheck } from '../queries/migrationJob'

const checkVariant = {
  pass: 'success',
  warn: 'warning',
  fail: 'destructive',
} as const

function Checks({ checks }: { checks: ProbeCheck[] }) {
  return (
    <ul className="space-y-1">
      {checks.map((c) => (
        <li key={c.id} className="flex flex-wrap items-start gap-2 text-sm">
          <Badge variant={checkVariant[c.status]}>{c.status}</Badge>
          <span className="flex-1 break-words">
            {c.detail}
            {c.fix ? (
              <span className="block text-muted-foreground">{c.fix}</span>
            ) : null}
          </span>
        </li>
      ))}
    </ul>
  )
}

// Two on-demand probes per domain: whether public DNS already points here, and whether the app
// answers from this server. The second one works before DNS has moved.
export function MigrationDomainProbes({
  sessionId,
  domain,
  targetIp,
}: {
  sessionId: string
  domain: string
  targetIp?: string
}) {
  const { t } = useTranslation('migration')
  const probe = useProbeDomain()
  const res = probe.data
  const checks: ProbeCheck[] = res
    ? [...(res.dns ? [res.dns] : []), ...(res.http ?? [])]
    : []
  return (
    <div className="mt-2 space-y-2">
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={probe.isPending}
          onClick={() =>
            probe.mutate({ id: sessionId, domain, kind: 'dns', targetIp })
          }
        >
          {probe.isPending
            ? t('appImport.probe.checking')
            : t('appImport.probe.dns')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={probe.isPending}
          onClick={() =>
            probe.mutate({ id: sessionId, domain, kind: 'http', targetIp })
          }
        >
          {t('appImport.probe.http')}
        </Button>
      </div>
      <div aria-live="polite" className="space-y-1">
        {probe.isError ? (
          <p className="text-xs text-destructive">{probe.error.message}</p>
        ) : null}
        {checks.length > 0 ? <Checks checks={checks} /> : null}
      </div>
    </div>
  )
}
