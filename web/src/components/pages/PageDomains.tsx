import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { appKeys } from '../../queries/apps'
import {
  addSiteDomain,
  fetchSiteDomainStatus,
  type SiteDomainSetup,
} from '../../queries/pagesSites'

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
        <Badge variant={current.verified ? 'success' : 'warning'}>
          {current.verified ? t('domains.dnsOk') : t('domains.dnsWaiting')}
        </Badge>
        <Badge variant={current.cert === 'issued' ? 'success' : 'warning'}>
          {current.cert === 'issued'
            ? t('domains.certIssued')
            : t('domains.certPending')}
        </Badge>
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
      {setup.mode === 'auto' ? (
        <p className="text-xs text-muted-foreground">
          {t('domains.createdForYou')}
        </p>
      ) : null}
      {setup.mode === 'manual' && setup.records.length > 0 ? (
        <table className="w-full text-left text-sm">
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
                <td className="max-w-40 pr-3">
                  <CopyCell value={r.name} label={t('domains.copyHost')} />
                </td>
                <td className="max-w-64">
                  <CopyCell value={r.value} label={t('domains.copyValue')} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
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

/** Compact "connect a domain" row plus the DNS setup of each domain added in this view. */
export function PageDomains({ name }: Readonly<{ name: string }>) {
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
  const step = latest ? 2 : 1
  return (
    <div className="space-y-3 rounded-lg border border-border bg-card p-3">
      <ol className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
        {(['step1', 'step2', 'step3'] as const).map((k, i) => (
          <li
            key={k}
            aria-current={i + 1 === step ? 'step' : undefined}
            className={
              i + 1 <= step
                ? 'font-medium text-foreground'
                : 'text-muted-foreground'
            }
          >
            {i + 1}. {t(`domains.${k}`)}
          </li>
        ))}
      </ol>
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
