import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DEMO_BUILD_LOG,
  DEMO_SAMPLE_LOGS,
  demoDnsRows,
  demoSampleConnection,
  demoSampleUrl,
  type DemoEnvDiff,
} from '@/lib/demoSamples'
import { DemoSampleFrame } from './DemoSampleFrame'

function Mono({ children, label }: { children: ReactNode; label: string }) {
  return (
    <pre
      role="group"
      aria-label={label}
      className="overflow-auto rounded-md border border-border bg-muted p-3 font-mono text-xs"
    >
      {children}
    </pre>
  )
}

/** DemoSampleAppResult stands in for the created app after the last wizard step. */
export function DemoSampleAppResult({
  name,
  onBack,
}: {
  name: string
  onBack?: () => void
}) {
  const { t } = useTranslation('demo')
  const url = demoSampleUrl(name)
  return (
    <DemoSampleFrame hint={t('sample.app.hint')}>
      <p className="text-sm font-medium">{t('sample.app.created', { name })}</p>
      <p className="text-sm text-muted-foreground">
        {t('sample.app.url')} <span className="font-mono">{url}</span>
      </p>
      <Mono label={t('sample.app.logLabel')}>
        {DEMO_BUILD_LOG.map((l, i) => (
          <div key={i}>{l}</div>
        ))}
      </Mono>
      {onBack ? (
        <div>
          <Button type="button" variant="outline" size="sm" onClick={onBack}>
            {t('sample.back')}
          </Button>
        </div>
      ) : null}
    </DemoSampleFrame>
  )
}

/** DemoSampleDatabaseResult shows a masked connection string and a sample status. */
export function DemoSampleDatabaseResult({
  name,
  engine,
  onClose,
}: {
  name: string
  engine: string
  onClose?: () => void
}) {
  const { t } = useTranslation('demo')
  return (
    <DemoSampleFrame hint={t('sample.db.hint')}>
      <p className="text-sm font-medium">{t('sample.db.created', { name })}</p>
      <p className="text-sm text-muted-foreground">
        {t('sample.db.status')}: {t('sample.db.running')}
      </p>
      <Mono label={t('sample.db.connectionLabel')}>
        {demoSampleConnection(engine, name)}
      </Mono>
      <p className="text-xs text-muted-foreground">{t('sample.db.masked')}</p>
      {onClose ? (
        <div>
          <Button type="button" variant="outline" size="sm" onClick={onClose}>
            {t('sample.close')}
          </Button>
        </div>
      ) : null}
    </DemoSampleFrame>
  )
}

/** DemoSampleEnvSaved lists which keys a save would touch, never their values. */
export function DemoSampleEnvSaved({ diff }: { diff: DemoEnvDiff }) {
  const { t } = useTranslation('demo')
  const rows: { sign: string; label: string; keys: string[] }[] = [
    { sign: '+', label: t('sample.env.added'), keys: diff.added },
    { sign: '~', label: t('sample.env.changed'), keys: diff.changed },
    { sign: '-', label: t('sample.env.removed'), keys: diff.removed },
  ]
  const any = rows.some((r) => r.keys.length > 0)
  return (
    <DemoSampleFrame hint={t('sample.env.hint')}>
      <div role="status" className="space-y-2 text-sm">
        <p className="font-medium">{t('sample.env.saved')}</p>
        {any ? (
          <ul className="space-y-1 font-mono text-xs">
            {rows
              .filter((r) => r.keys.length > 0)
              .map((r) => (
                <li key={r.sign}>
                  <span aria-hidden="true">{r.sign} </span>
                  <span className="font-sans text-muted-foreground">
                    {r.label}:{' '}
                  </span>
                  {r.keys.join(', ')}
                </li>
              ))}
          </ul>
        ) : (
          <p className="text-muted-foreground">{t('sample.env.noChanges')}</p>
        )}
        <p className="text-muted-foreground">{t('sample.env.redeploy')}</p>
      </div>
    </DemoSampleFrame>
  )
}

/** DemoSampleScale previews a replica change and its rollout without touching the app. */
export function DemoSampleScale({ replicas }: { replicas: number }) {
  const { t } = useTranslation('demo')
  const [target, setTarget] = useState(String(replicas + 1))
  const [shown, setShown] = useState<number | null>(null)
  const steps = ['start', 'health', 'route', 'done'] as const
  return (
    <DemoSampleFrame hint={t('sample.scale.hint')}>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          const n = Number(target)
          if (Number.isInteger(n) && n >= 0) setShown(n)
        }}
      >
        <div className="space-y-1">
          <Label htmlFor="demo-scale-target">{t('sample.scale.target')}</Label>
          <Input
            id="demo-scale-target"
            type="number"
            min={0}
            className="w-24"
            value={target}
            onChange={(e) => {
              setTarget(e.target.value)
            }}
          />
        </div>
        <Button type="submit" size="sm">
          {t('sample.scale.preview')}
        </Button>
      </form>
      {shown !== null ? (
        <div role="status" className="space-y-2 text-sm">
          <p className="font-medium">
            {t('sample.scale.change', { from: replicas, to: shown })}
          </p>
          <ol className="list-decimal space-y-0.5 pl-5 text-muted-foreground">
            {steps.map((s) => (
              <li key={s}>{t(`sample.scale.steps.${s}`)}</li>
            ))}
          </ol>
        </div>
      ) : null}
    </DemoSampleFrame>
  )
}

/** DemoSampleDnsCheck shows canned DNS rows for a domain, no lookup runs. */
export function DemoSampleDnsCheck() {
  const { t } = useTranslation('demo')
  const [domain, setDomain] = useState('app.example.com')
  const [checked, setChecked] = useState<string | null>(null)
  return (
    <DemoSampleFrame hint={t('sample.dns.hint')}>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setChecked(domain)
        }}
      >
        <div className="min-w-0 flex-1 space-y-1">
          <Label htmlFor="demo-dns-domain">{t('sample.dns.domain')}</Label>
          <Input
            id="demo-dns-domain"
            value={domain}
            onChange={(e) => {
              setDomain(e.target.value)
            }}
          />
        </div>
        <Button type="submit" size="sm">
          {t('sample.dns.check')}
        </Button>
      </form>
      {checked !== null ? (
        <div className="overflow-auto rounded-md border border-border">
          <table className="w-full text-left text-xs">
            <caption className="sr-only">{t('sample.dns.resultLabel')}</caption>
            <thead className="bg-muted text-muted-foreground">
              <tr>
                <th scope="col" className="px-3 py-1.5 font-medium">
                  {t('sample.dns.type')}
                </th>
                <th scope="col" className="px-3 py-1.5 font-medium">
                  {t('sample.dns.host')}
                </th>
                <th scope="col" className="px-3 py-1.5 font-medium">
                  {t('sample.dns.value')}
                </th>
                <th scope="col" className="px-3 py-1.5 font-medium">
                  {t('sample.dns.state')}
                </th>
              </tr>
            </thead>
            <tbody className="font-mono">
              {demoDnsRows(checked).map((r) => (
                <tr key={r.type} className="border-t border-border">
                  <td className="px-3 py-1.5">{r.type}</td>
                  <td className="px-3 py-1.5 break-all">{r.host}</td>
                  <td className="px-3 py-1.5 break-all">{r.value}</td>
                  <td className="px-3 py-1.5 font-sans">
                    {r.verified
                      ? t('sample.dns.verified')
                      : t('sample.dns.pending')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </DemoSampleFrame>
  )
}

/** DemoSampleLogs fills an empty app log with a few labeled lines. */
export function DemoSampleLogs() {
  const { t } = useTranslation('demo')
  return (
    <DemoSampleFrame hint={t('sample.logs.hint')}>
      <Mono label={t('sample.logs.label')}>
        {DEMO_SAMPLE_LOGS.map((l, i) => (
          <div key={i}>{l}</div>
        ))}
      </Mono>
    </DemoSampleFrame>
  )
}
