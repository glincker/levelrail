import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LinkGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import {
  resolveSelection,
  selectAllState,
  setKeys,
  toggleKey,
  type BatchAction,
} from '../../lib/domainsBatch'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { tlsDaysText, tlsTone } from '../../lib/domainTls'
import {
  useParkedDomains,
  type ParkedDns,
  type ParkedTls,
} from '../../queries/parkedDomains'
import { DomainsBatchDialog } from './DomainsBatchDialog'
import { DomainsBulkBar, RowSelectBox, SelectAllBox } from './DomainsBulkBar'
import {
  AttachParkedDialog,
  DeleteParkedDialog,
} from './DomainLifecycleDialogs'

const DNS_VARIANT = {
  pointing: 'success',
  not_pointing: 'warning',
  unknown: 'muted',
} as const

function ParkedDnsBadge({ dns }: { dns?: ParkedDns }) {
  const { t } = useTranslation('domains')
  if (!dns) return null
  return (
    <Badge variant={DNS_VARIANT[dns.status]}>
      {t(`parked.dns.${dns.status}`)}
    </Badge>
  )
}

function ParkedTlsBadge({ tls }: { tls?: ParkedTls }) {
  const { t } = useTranslation('domains')
  if (!tls) return null
  if (tls.state === 'pending') {
    return <Badge variant="info">{t('parked.tls.pending')}</Badge>
  }
  if (tls.state === 'none' || tls.days_left === undefined) {
    return <Badge variant="muted">{t('parked.tls.none')}</Badge>
  }
  const tone = tlsTone(tls.days_left)
  const { key, count } = tlsDaysText(tls.days_left)
  return (
    <Badge
      variant={
        tone === 'error'
          ? 'destructive'
          : tone === 'warn'
            ? 'warning'
            : 'success'
      }
      title={tls.not_after}
    >
      {t(key, { count })}
    </Badge>
  )
}

type Action = {
  kind: 'attach' | 'delete'
  domain: string
  hasSettings?: boolean
}

// Parked domains stay registered with DNS and certificate but route to no app.
export function ParkedDomainsGroup() {
  const { t } = useTranslation('domains')
  const { data: parked = [] } = useParkedDomains()
  const [action, setAction] = useState<Action | null>(null)
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set())
  const [batchAction, setBatchAction] = useState<BatchAction | null>(null)
  if (parked.length === 0) return null
  const keys = parked.map((p) => p.domain)
  const chosen = resolveSelection(
    selected,
    parked,
    (p) => p.domain,
    () => true,
  )
  return (
    <section aria-labelledby="parked-domains-heading" className="space-y-2">
      <div className="flex items-center gap-2">
        <h2 id="parked-domains-heading" className="text-sm font-medium">
          {t('parked.title')}
        </h2>
        <Badge variant="muted">{parked.length}</Badge>
        <span className="ml-auto">
          <SelectAllBox
            state={selectAllState(selected, keys)}
            onChange={(on) => {
              setSelected((s) => setKeys(s, keys, on))
            }}
          />
        </span>
      </div>
      <DomainsBulkBar
        count={chosen.length}
        actions={[
          {
            id: 'attach',
            label: t('bulk.attach'),
            icon: <LinkGlassIcon size="1.4em" aria-hidden="true" />,
            onClick: () => {
              setBatchAction('attach')
            },
          },
          {
            id: 'delete',
            label: t('bulk.delete'),
            destructive: true,
            icon: <TrashGlassIcon size="1.4em" aria-hidden="true" />,
            onClick: () => {
              setBatchAction('delete_parked')
            },
          },
        ]}
        onClear={() => {
          setSelected(new Set())
        }}
      />
      <p className="text-xs text-muted-foreground">{t('parked.hint')}</p>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {parked.map((p) => (
          <li
            key={p.domain}
            className="flex flex-wrap items-center gap-2 px-3 py-2"
          >
            <RowSelectBox
              domain={p.domain}
              checked={selected.has(p.domain)}
              onChange={() => {
                setSelected((s) => toggleKey(s, p.domain))
              }}
            />
            <span className="min-w-0 flex-1 truncate font-mono text-sm">
              {p.domain}
            </span>
            {p.parked_from ? (
              <span className="text-xs text-muted-foreground">
                {p.parked_from_environment
                  ? t('parked.fromEnvironment', {
                      app: p.parked_from,
                      environment: p.parked_from_environment,
                    })
                  : t('parked.from', { app: p.parked_from })}
              </span>
            ) : null}
            {p.parked_from_kind === 'static_site' ? (
              <Badge variant="outline">{t('parked.staticSite')}</Badge>
            ) : null}
            <ParkedDnsBadge dns={p.dns} />
            <ParkedTlsBadge tls={p.tls} />
            {p.has_settings ? (
              <Badge variant="info">{t('parked.settingsSaved')}</Badge>
            ) : null}
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setAction({
                  kind: 'attach',
                  domain: p.domain,
                  hasSettings: p.has_settings === true,
                })
              }}
            >
              <LinkGlassIcon size="1.4em" aria-hidden="true" />
              {t('parked.attach')}
            </Button>
            <Button
              size="icon-sm"
              variant="ghost"
              onClick={() => {
                setAction({ kind: 'delete', domain: p.domain })
              }}
            >
              <TrashGlassIcon size="1.4em" aria-hidden="true" />
              <span className="sr-only">
                {t('parked.delete', { domain: p.domain })}
              </span>
            </Button>
          </li>
        ))}
      </ul>
      {action?.kind === 'attach' ? (
        <AttachParkedDialog
          domain={action.domain}
          hasSettings={action.hasSettings}
          open
          onOpenChange={(o) => {
            if (!o) setAction(null)
          }}
        />
      ) : null}
      {action?.kind === 'delete' ? (
        <DeleteParkedDialog
          domain={action.domain}
          open
          onOpenChange={(o) => {
            if (!o) setAction(null)
          }}
        />
      ) : null}
      {batchAction ? (
        <DomainsBatchDialog
          action={batchAction}
          subjects={chosen.map((p) => ({
            domain: p.domain,
            service_name: p.parked_from ?? '',
            has_settings: p.has_settings,
          }))}
          open
          onOpenChange={(o) => {
            if (!o) setBatchAction(null)
          }}
          onDone={(ok) => {
            setSelected((s) => setKeys(s, ok, false))
          }}
        />
      ) : null}
    </section>
  )
}
