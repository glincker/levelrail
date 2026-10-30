import { useEffect, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { domainsQueryOptions } from '@/queries/domains'
import {
  applyMailDns,
  checkMailDns,
  planMailDns,
  type MailCheckReport,
  type MailDnsSpec,
  type MailPlan,
  type MailPlanAction,
} from '@/queries/mailDns'
import { StatusGlyph } from './MailChecklist'

const SELF = 'self'
// Public preset ids from internal/mailrecords/presets.go; the API owns the values.
const PRESETS = [
  ['google', 'Google Workspace'],
  ['fastmail', 'Fastmail'],
  ['proton', 'Proton Mail'],
  ['zoho', 'Zoho Mail'],
  ['ses', 'Amazon SES'],
  ['mailgun', 'Mailgun'],
  ['postmark', 'Postmark'],
] as const

// Which check item grades a planned record kind.
const KIND_CHECK: Record<string, string> = {
  MX: 'mx',
  SPF: 'spf',
  DKIM: 'dkim',
  DMARC: 'dmarc',
  MTASTS: 'mta_sts',
  TLSRPT: 'tls_rpt',
  AUTOCONFIG: 'autoconfig',
}

const ACTION_VARIANT: Record<
  MailPlanAction,
  'success' | 'muted' | 'warning' | 'destructive'
> = {
  add: 'success',
  unchanged: 'muted',
  replace: 'warning',
  refused: 'destructive',
}

const errText = (e: unknown) => (e instanceof Error ? e.message : '')

/** MailDnsPanel plans, applies (pinned by plan hash) and checks the mail records of one domain. */
export function MailDnsPanel({
  report,
  onReport,
  onDomain,
}: {
  report: MailCheckReport | undefined
  onReport: (r: MailCheckReport) => void
  onDomain?: (domain: string) => void
}) {
  const { t } = useTranslation('mailServer')
  const domains = useQuery(domainsQueryOptions())
  const known = [
    ...new Set(
      (domains.data ?? []).filter((d) => !d.automatic).map((d) => d.domain),
    ),
  ]
  const [domain, setDomain] = useState<string | null>(null)
  const [preset, setPreset] = useState(SELF)
  const [host, setHost] = useState<string | null>(null)
  const [ip, setIp] = useState('')
  const [plan, setPlan] = useState<MailPlan | null>(null)

  const effective = (domain ?? known[0] ?? '').trim().toLowerCase()
  useEffect(() => {
    onDomain?.(effective)
  }, [effective, onDomain])
  const mailHost = host ?? (effective ? `mail.${effective}` : '')
  const spec: MailDnsSpec =
    preset === SELF
      ? { domain: effective, mail_host: mailHost, ipv4: ip.trim() || undefined }
      : { domain: effective, preset }

  const planMut = useMutation({
    mutationFn: planMailDns,
    onSuccess: setPlan,
    onError: (e) =>
      toast.add({
        title: t('dns.error', { message: errText(e) }),
        type: 'error',
      }),
  })
  const applyMut = useMutation({
    mutationFn: (hash: string) => applyMailDns(spec, hash),
    onSuccess: (res) => {
      toast.add({
        title: t('dns.applied', { count: res.applied }),
        type: 'success',
      })
      setPlan(res.plan)
    },
    onError: (e) =>
      toast.add({
        title: t('dns.error', { message: errText(e) }),
        type: 'error',
      }),
  })
  const checkMut = useMutation({
    mutationFn: checkMailDns,
    onSuccess: onReport,
    onError: (e) =>
      toast.add({
        title: t('dns.error', { message: errText(e) }),
        type: 'error',
      }),
  })

  const touch = () => setPlan(null)
  const pending =
    plan?.changes.filter((c) => c.action === 'add' || c.action === 'replace') ??
    []
  const canApply = !!plan && !plan.blocked && pending.length > 0
  const liveFor = (kind: string) => {
    const id = KIND_CHECK[kind]
    return id ? report?.items.find((i) => i.id === id) : undefined
  }

  return (
    <section
      aria-label={t('dns.title')}
      className="space-y-3 rounded-lg border border-border bg-card p-3"
    >
      <h2 className="text-sm font-medium">{t('dns.title')}</h2>
      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (effective) planMut.mutate(spec)
        }}
      >
        <div className="space-y-1">
          <Label htmlFor="mail-domain">{t('dns.domain')}</Label>
          <Input
            id="mail-domain"
            list="mail-known-domains"
            className="w-52"
            value={domain ?? effective}
            placeholder={t('dns.domainPlaceholder')}
            onChange={(e) => {
              setDomain(e.target.value)
              setHost(null)
              touch()
            }}
          />
          <datalist id="mail-known-domains" aria-label={t('dns.knownDomains')}>
            {known.map((d) => (
              <option key={d} value={d} />
            ))}
          </datalist>
        </div>
        <div className="space-y-1">
          <Label htmlFor="mail-preset">{t('dns.preset')}</Label>
          <Select
            value={preset}
            onValueChange={(v) => {
              if (v) setPreset(v)
              touch()
            }}
          >
            <SelectTrigger id="mail-preset" className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={SELF}>{t('dns.selfHosted')}</SelectItem>
              {PRESETS.map(([id, name]) => (
                <SelectItem key={id} value={id}>
                  {name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {preset === SELF && (
          <>
            <div className="space-y-1">
              <Label htmlFor="mail-host">{t('dns.mailHost')}</Label>
              <Input
                id="mail-host"
                className="w-48"
                value={mailHost}
                onChange={(e) => {
                  setHost(e.target.value)
                  touch()
                }}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="mail-ip">{t('dns.ipv4')}</Label>
              <Input
                id="mail-ip"
                className="w-40"
                inputMode="decimal"
                value={ip}
                placeholder={t('dns.ipv4Placeholder')}
                onChange={(e) => {
                  setIp(e.target.value)
                  touch()
                }}
              />
            </div>
          </>
        )}
        <Button
          type="submit"
          variant="outline"
          disabled={!effective || planMut.isPending}
        >
          {t('dns.plan')}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!effective || checkMut.isPending}
          onClick={() => checkMut.mutate(spec)}
        >
          {checkMut.isPending ? t('dns.checking') : t('dns.check')}
        </Button>
      </form>

      {plan && (
        <div className="space-y-2">
          {plan.blocked && (
            <p role="alert" className="text-xs text-tone-danger">
              {t('dns.blocked')}
            </p>
          )}
          {pending.length === 0 && !plan.blocked && (
            <p className="text-xs text-muted-foreground">
              {t('dns.noChanges')}
            </p>
          )}
          <div className="overflow-x-auto">
            <table
              aria-label={t('dns.tableLabel')}
              className="w-full text-left text-xs"
            >
              <thead className="text-muted-foreground">
                <tr>
                  <th scope="col" className="py-1 pr-3 font-medium">
                    {t('dns.colAction')}
                  </th>
                  <th scope="col" className="py-1 pr-3 font-medium">
                    {t('dns.colType')}
                  </th>
                  <th scope="col" className="py-1 pr-3 font-medium">
                    {t('dns.colName')}
                  </th>
                  <th scope="col" className="py-1 pr-3 font-medium">
                    {t('dns.colValue')}
                  </th>
                  <th scope="col" className="py-1 font-medium">
                    {t('dns.colLive')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {plan.changes.map((c, i) => {
                  const live = liveFor(c.kind)
                  return (
                    <tr
                      key={`${c.set.name}-${c.set.type}-${i}`}
                      className="border-t border-border align-top"
                    >
                      <td className="py-1 pr-3">
                        <Badge variant={ACTION_VARIANT[c.action]}>
                          {t(`dns.action.${c.action}`)}
                        </Badge>
                      </td>
                      <td className="py-1 pr-3 font-mono">{c.set.type}</td>
                      <td className="py-1 pr-3 font-mono">{c.set.name}</td>
                      <td className="max-w-md break-all py-1 pr-3 font-mono text-muted-foreground">
                        {c.set.values.join(' | ')}
                      </td>
                      <td className="py-1">
                        <span className="inline-flex items-center gap-1">
                          <StatusGlyph status={live?.status ?? 'not_tested'} />
                          <span className="sr-only">
                            {t(`status.${live?.status ?? 'not_tested'}`)}
                          </span>
                        </span>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              disabled={!canApply || applyMut.isPending}
              onClick={() => applyMut.mutate(plan.hash)}
            >
              {applyMut.isPending
                ? t('dns.applying')
                : t('dns.apply', { count: pending.length })}
            </Button>
            <span className="font-mono text-xs text-muted-foreground">
              {t('dns.planHash', { hash: plan.hash.slice(0, 12) })}
            </span>
          </div>
        </div>
      )}
    </section>
  )
}
