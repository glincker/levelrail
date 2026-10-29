import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  ChevronDownGlassIcon,
  CircleAlertGlassIcon,
  CopyGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { STANDALONE_SCOPE } from '../../lib/standaloneDomain'
import { domainCheckQueryOptions } from '../../queries/domainCheck'
import {
  useServerAddress,
  useSetServerAddress,
  type DnsRecordSuggestion,
  type ServerAddress,
} from '../../queries/serverAddress'
import { DomainDnsStatusBadge } from '../DomainDnsStatusBadge'

function CopyValue({ value }: { value: string }) {
  const { t } = useTranslation('domains')
  const { copied, copy } = useCopyToClipboard()
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      aria-label={
        copied ? t('serverAddress.copied') : t('serverAddress.copy', { value })
      }
      onClick={() => {
        copy(value)
      }}
    >
      {copied ? (
        <CheckGlassIcon size="1.2em" />
      ) : (
        <CopyGlassIcon size="1.2em" />
      )}
    </Button>
  )
}

function AddressRow({ label, value }: { label: string; value?: string }) {
  const { t } = useTranslation('domains')
  return (
    <div className="flex min-w-0 items-center gap-2">
      <span className="w-10 shrink-0 text-xs text-muted-foreground">
        {label}
      </span>
      {value ? (
        <>
          <span className="min-w-0 truncate font-mono text-sm">{value}</span>
          <CopyValue value={value} />
        </>
      ) : (
        <span className="text-sm text-muted-foreground">
          {t('serverAddress.none')}
        </span>
      )}
    </div>
  )
}

function Notes({ data }: { data: ServerAddress }) {
  const { t } = useTranslation('domains')
  const notes: string[] = []
  if (!data.ipv4 && !data.ipv6) notes.push(t('serverAddress.noneDetected'))
  if (data.behind_nat) notes.push(t('serverAddress.nat'))
  if (data.tls_terminated_upstream) notes.push(t('serverAddress.upstream'))
  if (notes.length === 0) return null
  return (
    <ul className="space-y-1">
      {notes.map((n) => (
        <li
          key={n}
          className="flex items-start gap-1.5 text-xs text-amber-700 dark:text-amber-400"
        >
          <CircleAlertGlassIcon
            className="mt-0.5 size-4 shrink-0 text-tone-danger"
            aria-hidden="true"
          />
          <span>{n}</span>
        </li>
      ))}
    </ul>
  )
}

function OverrideForm({ data }: { data: ServerAddress }) {
  const { t } = useTranslation('domains')
  const save = useSetServerAddress()
  const [ipv4, setIpv4] = useState(data.override.ipv4 ?? '')
  const [ipv6, setIpv6] = useState(data.override.ipv6 ?? '')
  const hasOverride = Boolean(data.override.ipv4 || data.override.ipv6)
  function submit(next: { ipv4: string; ipv6: string }) {
    save.mutate(next, {
      onSuccess: () => {
        toast.add({ title: t('serverAddress.saved'), type: 'success' })
      },
      onError: (err) => {
        toast.add({
          title: t('serverAddress.saveFailed'),
          description: err.message,
          type: 'error',
        })
      },
    })
  }
  return (
    <Collapsible>
      <CollapsibleTrigger className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
        <ChevronDownGlassIcon className="size-3.5" aria-hidden="true" />
        {t('serverAddress.override')}
      </CollapsibleTrigger>
      <CollapsiblePanel className="space-y-2 pt-2">
        <div className="grid gap-2 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="server-address-ipv4">
              {t('serverAddress.ipv4')}
            </FieldLabel>
            <Input
              id="server-address-ipv4"
              className="font-mono"
              value={ipv4}
              placeholder={data.detected.ipv4 ?? '203.0.113.10'}
              onChange={(e) => {
                setIpv4(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="server-address-ipv6">
              {t('serverAddress.ipv6')}
            </FieldLabel>
            <Input
              id="server-address-ipv6"
              className="font-mono"
              value={ipv6}
              placeholder={data.detected.ipv6 ?? '2001:db8::10'}
              onChange={(e) => {
                setIpv6(e.target.value)
              }}
            />
          </Field>
        </div>
        <FieldHint>{t('serverAddress.overrideHint')}</FieldHint>
        <div className="flex gap-2">
          <Button
            type="button"
            size="sm"
            disabled={save.isPending || (!ipv4.trim() && !ipv6.trim())}
            onClick={() => {
              submit({ ipv4: ipv4.trim(), ipv6: ipv6.trim() })
            }}
          >
            {t('serverAddress.save')}
          </Button>
          {hasOverride ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={save.isPending}
              onClick={() => {
                setIpv4('')
                setIpv6('')
                submit({ ipv4: '', ipv6: '' })
              }}
            >
              {t('serverAddress.clear')}
            </Button>
          ) : null}
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}

export function DnsRecordsTable({
  records,
}: {
  records: DnsRecordSuggestion[]
}) {
  const { t } = useTranslation('domains')
  if (records.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        {t('serverAddress.recordsNone')}
      </p>
    )
  }
  return (
    <div className="overflow-x-auto rounded-md border border-border">
      <table className="w-full text-left text-xs">
        <thead className="text-muted-foreground">
          <tr>
            <th className="px-2 py-1 font-medium">
              {t('serverAddress.recordType')}
            </th>
            <th className="px-2 py-1 font-medium">
              {t('serverAddress.recordName')}
            </th>
            <th className="px-2 py-1 font-medium">
              {t('serverAddress.recordValue')}
            </th>
          </tr>
        </thead>
        <tbody>
          {records.map((r) => (
            <tr key={`${r.type}-${r.value}`} className="border-t border-border">
              <td className="px-2 py-1 font-mono">
                {r.alternative ? (
                  <span className="mr-1 text-muted-foreground">
                    {t('serverAddress.orInstead')}
                  </span>
                ) : null}
                {r.type}
              </td>
              <td className="px-2 py-1 font-mono break-all">{r.name}</td>
              <td className="px-2 py-1 font-mono">
                <span className="inline-flex items-center gap-1 break-all">
                  {r.value}
                  <CopyValue value={r.value} />
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// DomainRecords shows the exact record for domain plus its live DNS status.
export function DomainRecords({ domain }: { domain: string }) {
  const { t } = useTranslation('domains')
  const { data } = useServerAddress(domain, domain.length > 0)
  const check = useQuery({
    ...domainCheckQueryOptions(STANDALONE_SCOPE, domain),
    staleTime: 30_000,
  })
  if (!domain || !data) return null
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs font-medium">{t('serverAddress.recordsTitle')}</p>
        <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
          {t('serverAddress.dnsStatus')}
          <DomainDnsStatusBadge status={check.data?.status} />
        </span>
      </div>
      <DnsRecordsTable records={data.records ?? []} />
    </div>
  )
}

// ServerAddressLine is the compact form for dialogs: both IPs with copy buttons.
export function ServerAddressLine() {
  const { t } = useTranslation('domains')
  const { data } = useServerAddress()
  if (!data) return null
  return (
    <div className="space-y-1 rounded-md border border-border px-3 py-2">
      <p className="text-xs font-medium">{t('serverAddress.title')}</p>
      <div className="flex flex-wrap gap-x-6">
        <AddressRow label={t('serverAddress.ipv4')} value={data.ipv4} />
        <AddressRow label={t('serverAddress.ipv6')} value={data.ipv6} />
      </div>
      <Notes data={data} />
    </div>
  )
}

export function ServerAddressCard() {
  const { t } = useTranslation('domains')
  const { data, isLoading } = useServerAddress()
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ServerGlassIcon className="size-4.5" aria-hidden="true" />
          {t('serverAddress.title')}
          {data ? (
            <Badge variant="muted">
              {t(`serverAddress.source.${data.source}`)}
            </Badge>
          ) : null}
        </CardTitle>
        <CardDescription>{t('serverAddress.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading || !data ? (
          <Skeleton className="h-10 w-64" />
        ) : (
          <>
            <div className="flex flex-wrap gap-x-8 gap-y-1">
              <AddressRow label={t('serverAddress.ipv4')} value={data.ipv4} />
              <AddressRow label={t('serverAddress.ipv6')} value={data.ipv6} />
            </div>
            <Notes data={data} />
            <OverrideForm
              key={`${data.override.ipv4 ?? ''}-${data.override.ipv6 ?? ''}`}
              data={data}
            />
          </>
        )}
      </CardContent>
    </Card>
  )
}
