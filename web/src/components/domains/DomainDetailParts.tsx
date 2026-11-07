import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  ClockGlassIcon,
  ExternalLinkGlassIcon,
  SendGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { SectionTitle } from '@/components/ui/typography'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { PROVIDER_DNS_URL, detectNsProvider } from '../../lib/dnsProviderHint'
import { DnsRecordRow, type DomainCheckPanelData } from '../DomainDnsCheck'
import {
  dnsRecordOptions,
  type DnsRecordType,
} from '../../lib/dnsRecordOptions'
import {
  nameserverNames,
  openHost,
  resolverChips,
  wrongValue,
  type ChipState,
} from './domainDetailModel'
import {
  DeleteDomainDialog,
  MoveDomainDialog,
  ParkDomainDialog,
} from './DomainLifecycleDialogs'

export function DetailCard({
  title,
  children,
  tone,
}: {
  title: string
  children: ReactNode
  tone?: 'danger'
}) {
  return (
    <section
      className={cn(
        'space-y-3 rounded-lg border bg-card p-4',
        tone === 'danger' ? 'border-destructive/40' : 'border-border',
      )}
    >
      <SectionTitle as="h2">{title}</SectionTitle>
      {children}
    </section>
  )
}

const CHIP_ICON: Record<ChipState, typeof CheckGlassIcon> = {
  ok: CircleCheckGlassIcon,
  pending: ClockGlassIcon,
  wrong: CircleAlertGlassIcon,
}
const CHIP_VARIANT = {
  ok: 'success',
  pending: 'muted',
  wrong: 'warning',
} as const

// One small chip per place that answers DNS, instead of a text dump.
export function ResolverChips({ data }: { data: DomainCheckPanelData }) {
  const { t } = useTranslation('domains')
  return (
    <ul
      className="flex flex-wrap items-center gap-1.5"
      aria-label={t('detail.resolvers.label')}
    >
      {resolverChips(data).map((c) => {
        const Icon = CHIP_ICON[c.state]
        return (
          <li key={c.id}>
            <Badge variant={CHIP_VARIANT[c.state]}>
              <Icon className="size-3.5" aria-hidden="true" />
              {c.id === 'authoritative'
                ? t('detail.resolvers.authoritative')
                : c.id}
              <span className="sr-only">
                {t(`detail.resolvers.state.${c.state}`)}
              </span>
            </Badge>
          </li>
        )
      })}
    </ul>
  )
}

// Provider hint from the nameserver names, with a generic fallback.
function ProviderHint({ data }: { data: DomainCheckPanelData }) {
  const { t } = useTranslation('domains')
  const provider = detectNsProvider(nameserverNames(data))
  if (provider === 'generic')
    return (
      <p className="text-xs text-muted-foreground">
        {t('detail.provider.generic')}
      </p>
    )
  return (
    <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
      <span>{t(`detail.provider.${provider}`)}</span>
      <a
        href={PROVIDER_DNS_URL[provider]}
        target="_blank"
        rel="noreferrer noopener"
        className="inline-flex items-center gap-1 text-foreground underline underline-offset-2"
      >
        {t('detail.provider.open', {
          name: t(`detail.provider.name.${provider}`),
        })}
        <ExternalLinkGlassIcon className="size-4" aria-hidden="true" />
      </a>
    </p>
  )
}

// The record to add, copyable per field, plus a ready message for whoever runs DNS.
export function DnsRecordCard({
  domain,
  data,
}: {
  domain: string
  data: DomainCheckPanelData
}) {
  const { t } = useTranslation('domains')
  const options = dnsRecordOptions(data)
  const [type, setType] = useState<DnsRecordType | undefined>(options[0]?.type)
  const active = options.find((o) => o.type === type) ?? options[0]
  const { copied, copy } = useCopyToClipboard()
  const wrong = wrongValue(data)
  if (!active) return null
  return (
    <div className="space-y-2">
      {wrong ? (
        <p className="flex items-start gap-1.5 text-sm text-foreground">
          <CircleAlertGlassIcon
            className="mt-0.5 size-5 shrink-0 text-tone-warning"
            aria-hidden="true"
          />
          {t('detail.wrongValue', wrong)}
        </p>
      ) : null}
      {options.length > 1 ? (
        <div
          role="group"
          aria-label={t('detail.recordType')}
          className="flex gap-1"
        >
          {options.map((o) => (
            <Button
              key={o.type}
              type="button"
              size="xs"
              variant={o.type === active.type ? 'secondary' : 'ghost'}
              aria-pressed={o.type === active.type}
              onClick={() => {
                setType(o.type)
              }}
            >
              {o.type}
            </Button>
          ))}
        </div>
      ) : null}
      <div className="grid gap-1.5 lg:grid-cols-[6rem_minmax(0,1fr)_minmax(0,1.4fr)]">
        <DnsRecordRow label={t('detail.field.type')} value={active.type} />
        <DnsRecordRow label={t('detail.field.name')} value={domain} />
        <DnsRecordRow label={t('detail.field.value')} value={active.value} />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <ProviderHint data={data} />
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            copy(
              t('detail.adminMessage', {
                domain,
                type: active.type,
                value: active.value,
              }),
            )
          }}
        >
          {copied ? (
            <CheckGlassIcon aria-hidden="true" />
          ) : (
            <SendGlassIcon aria-hidden="true" />
          )}
          {copied ? t('detail.adminCopied') : t('detail.sendAdmin')}
        </Button>
      </div>
    </div>
  )
}

export interface TimelineItem {
  id: 'dns' | 'requested' | 'issued' | 'expires'
  done: boolean
  detail?: string
}

// Shown once DNS resolves: what happened to the certificate and when it ends.
export function CertTimeline({ items }: { items: TimelineItem[] }) {
  const { t } = useTranslation('domains')
  return (
    <ol
      className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4"
      aria-label={t('detail.timeline.label')}
    >
      {items.map((i) => (
        <li key={i.id} className="flex items-start gap-1.5 text-xs">
          {i.done ? (
            <CircleCheckGlassIcon
              className="size-5 shrink-0 text-tone-success"
              aria-hidden="true"
            />
          ) : (
            <ClockGlassIcon
              className="size-5 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          )}
          <span className="min-w-0">
            <span className="block font-medium text-foreground">
              {t(`detail.timeline.${i.id}`)}
            </span>
            <span className="block text-muted-foreground">
              {i.detail ??
                (i.done
                  ? t('detail.timeline.done')
                  : t('detail.timeline.todo'))}
            </span>
          </span>
        </li>
      ))}
    </ol>
  )
}

export function TestItRow({ domain }: { domain: string }) {
  const { t } = useTranslation('domains')
  const host = openHost(domain)
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <span className="font-medium">{t('detail.testIt')}</span>
      {(['http', 'https'] as const).map((scheme) => (
        <a
          key={scheme}
          href={`${scheme}://${host}`}
          target="_blank"
          rel="noreferrer noopener"
          className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
        >
          {scheme}://{host}
          <ExternalLinkGlassIcon aria-hidden="true" />
        </a>
      ))}
    </div>
  )
}

type Tool = 'park' | 'move' | 'delete'

export function DangerZone({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const [tool, setTool] = useState<Tool | null>(null)
  const close = (open: boolean) => {
    if (!open) setTool(null)
  }
  return (
    <DetailCard title={t('detail.danger.title')} tone="danger">
      <p className="text-sm text-muted-foreground">{t('detail.danger.body')}</p>
      <div className="flex flex-wrap gap-2">
        {(['move', 'park', 'delete'] as const).map((k) => (
          <Button
            key={k}
            type="button"
            size="sm"
            variant={k === 'delete' ? 'destructive' : 'outline'}
            onClick={() => {
              setTool(k)
            }}
          >
            {t(`page.row.${k}`)}
          </Button>
        ))}
      </div>
      {tool === 'park' ? (
        <ParkDomainDialog
          app={appName}
          domain={domain}
          open
          onOpenChange={close}
        />
      ) : null}
      {tool === 'move' ? (
        <MoveDomainDialog
          from={appName}
          domain={domain}
          open
          onOpenChange={close}
        />
      ) : null}
      {tool === 'delete' ? (
        <DeleteDomainDialog
          app={appName}
          domain={domain}
          open
          onOpenChange={close}
        />
      ) : null}
    </DetailCard>
  )
}
