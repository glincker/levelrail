import { lazy, Suspense, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CopyGlassIcon,
  GlobeGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { DomainStepper } from './DomainStepper'
import { CertTruthBadge } from '../CertTruthBadge'
import { ProxyHandledTlsBadge } from '../ProxyDomainStatus'
import { appKeys, useUpdateApp } from '../../queries/apps'
import { useCertificates } from '../../queries/certificates'
import type { AppDetail } from '../../types/appDetail'
import {
  addSiteDomain,
  fetchSiteDomainStatus,
  type SiteDomainSetup,
} from '../../queries/pagesSites'

const DomainChecklist = lazy(() => import('./DomainChecklist'))

// Poll cadence while a domain waits for DNS; tunable per build.
const POLL_MS = Number(import.meta.env.VITE_PAGES_DOMAIN_POLL_MS ?? 5000)

function CopyCell({
  value,
  label,
}: Readonly<{ value: string; label: string }>) {
  const { copied, copy } = useCopyToClipboard()
  return (
    <span className="flex min-w-0 items-center gap-1">
      <code className="truncate font-mono text-xs">{value}</code>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={label}
        onClick={() => copy(value)}
      >
        {copied ? (
          <CircleCheckGlassIcon className="size-4" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-4" aria-hidden="true" />
        )}
      </Button>
    </span>
  )
}

function SetupCard({
  name,
  setup,
}: Readonly<{ name: string; setup: SiteDomainSetup }>) {
  const { t } = useTranslation('pagesSites')
  const [polling, setPolling] = useState(false)
  const status = useQuery({
    queryKey: ['static-sites', 'domain-status', name, setup.domain],
    queryFn: () => fetchSiteDomainStatus(name, setup.domain),
    enabled: polling,
    retry: false,
    refetchInterval: (q) =>
      q.state.data?.verified && q.state.data.cert === 'issued'
        ? false
        : POLL_MS,
  })
  const current = status.data ?? setup
  const done = current.verified && current.cert === 'issued'

  return (
    <div className="space-y-2 border-t border-border pt-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-sm font-medium">{setup.domain}</span>
        <Badge variant="outline">{t(`domains.mode.${setup.mode}`)}</Badge>
        <DomainStepper
          dnsOk={current.verified}
          certOk={current.cert === 'issued'}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="ml-auto"
          disabled={polling || done}
          onClick={() => setPolling(true)}
        >
          {polling && !done ? t('domains.checking') : t('domains.verify')}
        </Button>
      </div>
      {setup.mode === 'manual' && setup.records.length > 0 ? (
        <table className="w-full table-fixed text-left text-sm">
          <caption className="sr-only">{t('domains.recordsLabel')}</caption>
          <thead className="text-xs text-muted-foreground">
            <tr>
              <th className="pr-3 font-normal">{t('domains.type')}</th>
              <th className="pr-3 font-normal">{t('domains.host')}</th>
              <th className="font-normal">{t('domains.value')}</th>
            </tr>
          </thead>
          <tbody>
            {setup.records.map((r) => (
              <tr key={`${r.type}-${r.name}-${r.value}`}>
                <td className="pr-3 font-mono text-xs">{r.type}</td>
                <td className="pr-3">
                  <CopyCell value={r.name} label={t('domains.copyHost')} />
                </td>
                <td>
                  <CopyCell value={r.value} label={t('domains.copyValue')} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
      {status.data?.diagnosis?.length ? (
        <Suspense fallback={null}>
          <DomainChecklist rows={status.data.diagnosis} />
        </Suspense>
      ) : null}
      {current.detail && !done ? (
        <p className="text-xs text-muted-foreground" aria-live="polite">
          {current.detail}
        </p>
      ) : null}
      {status.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('domains.statusFailed')}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}

function DomainList({ app }: Readonly<{ app: AppDetail }>) {
  const { t } = useTranslation('pagesSites')
  const { data: certificates } = useCertificates()
  const update = useUpdateApp(app.name, { inlineError: true })
  const domains = app.domains ?? []
  if (domains.length === 0) {
    return (
      <EmptyState
        compact
        icon={<GlobeGlassIcon className="size-6" />}
        title={t('domains.none')}
        action={
          <Button
            type="button"
            size="sm"
            onClick={() => document.getElementById('pages-domain')?.focus()}
          >
            {t('domains.add')}
          </Button>
        }
      />
    )
  }
  return (
    <ul className="divide-y divide-border rounded-md border border-border">
      {domains.map((d) => {
        const cert = certificates.find((c) => c.domain === d)
        return (
          <li
            key={d}
            className="flex flex-wrap items-center gap-x-4 gap-y-1 px-3 py-2"
          >
            <a
              href={`https://${d}`}
              target="_blank"
              rel="noreferrer"
              className="font-mono text-sm underline-offset-2 hover:underline focus-visible:underline"
            >
              {d}
            </a>
            <DomainStepper dnsOk={Boolean(cert)} certOk={Boolean(cert)} />
            {cert ? (
              <CertTruthBadge cert={cert} />
            ) : (
              <ProxyHandledTlsBadge
                domain={d}
                fallback={
                  <Badge variant="muted">{t('domains.provisioning')}</Badge>
                }
              />
            )}
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              className="ml-auto"
              disabled={update.isPending}
              aria-label={t('domains.remove', { domain: d })}
              onClick={() =>
                update.mutate({
                  ...app,
                  domains: domains.filter((x) => x !== d),
                })
              }
            >
              <XGlassIcon className="size-4" aria-hidden="true" />
            </Button>
          </li>
        )
      })}
      {update.isError ? (
        <li className="px-3 py-1.5 text-xs text-destructive" role="alert">
          {update.error.message}
        </li>
      ) : null}
    </ul>
  )
}

/** One card for a site's domains: current list with TLS and remove, then the three-step add flow. */
export function PageDomains({ app }: Readonly<{ app: AppDetail }>) {
  const name = app.name
  const { t } = useTranslation('pagesSites')
  const queryClient = useQueryClient()
  const [domain, setDomain] = useState('')
  const [setups, setSetups] = useState<SiteDomainSetup[]>([])
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const setup = await addSiteDomain(name, domain.trim().toLowerCase())
      setSetups((prev) => [
        setup,
        ...prev.filter((s) => s.domain !== setup.domain),
      ])
      setDomain('')
      await queryClient.invalidateQueries({ queryKey: appKeys.detail(name) })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('domains.addFailed'))
    } finally {
      setBusy(false)
    }
  }

  const latest = setups[0]
  return (
    <div className="space-y-3 rounded-lg border border-border bg-card p-3">
      <DomainList app={app} />
      <form
        onSubmit={(e) => void submit(e)}
        className="flex flex-wrap items-end gap-2"
      >
        <div className="min-w-48 flex-1 space-y-1.5">
          <Label htmlFor="pages-domain">{t('domains.label')}</Label>
          <Input
            id="pages-domain"
            value={domain}
            placeholder={t('domains.placeholder')}
            disabled={busy}
            onChange={(e) => setDomain(e.target.value)}
          />
        </div>
        <Button type="submit" size="sm" disabled={busy || domain.trim() === ''}>
          {busy ? t('domains.adding') : t('domains.add')}
        </Button>
      </form>
      {error ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {latest ? (
        <SetupCard key={latest.domain} name={name} setup={latest} />
      ) : null}
    </div>
  )
}
