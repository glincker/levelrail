import { useCallback, useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  MailGlassIcon,
  PlusGlassIcon,
  RocketGlassIcon,
  SettingsGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { cn } from '@/lib/utils'
import type { MailCheckReport } from '@/queries/mailDns'
import { isMailUp, useMailStatus } from '@/queries/mailStatus'
import { useQuery } from '@tanstack/react-query'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  isHiddenQueueError,
  useMailDomains,
  useMailQueue,
} from '../../queries/mailMailboxes'
import { teamListQueryOptions } from '../../queries/teams'
import { MailBackupLine } from './MailBackupLine'
import { MailDemoBadge } from './MailDemoBadge'
import { MailDnsPanel } from './MailDnsPanel'
import { MailDomainClaim } from './MailDomainClaim'
import { MailHoldBanner } from './MailHoldBanner'
import { MailMailboxes } from './MailMailboxes'
import { MailOverview } from './MailOverview'
import { MailQueueSheet } from './MailQueueSheet'
import { MailSettingsSheet } from './MailSettingsSheet'
import { MailWebmailButton } from './MailWebmailButton'
import {
  MAIL_TABS,
  parseMailTab,
  readStoredMailTab,
  storeMailTab,
  type MailTab,
} from './mailTabs'

interface Props {
  tab?: MailTab
  onTab?: (tab: MailTab) => void
  add?: boolean
  queue?: boolean
  /** Called once the add or queue deep link has been acted on, so it can repeat. */
  onHandled?: () => void
}

/** MailPage is the mail hub: a slim status header, four sections and sheets for the rest. */
export function MailPage({ tab, onTab, add, queue, onHandled }: Props = {}) {
  const { t } = useTranslation('mailServer')
  const [report, setReport] = useState<MailCheckReport | undefined>()
  const [domain, setDomain] = useState('')
  const [previewToken, setPreviewToken] = useState(0)
  const [local, setLocal] = useState<MailTab>(
    () => tab ?? readStoredMailTab() ?? 'overview',
  )
  const [adding, setAdding] = useState(!!add)
  const [settings, setSettings] = useState(false)
  const [queueOpen, setQueueOpen] = useState(!!queue)
  const statusQ = useMailStatus()
  const status = statusQ.data
  const up = isMailUp(status)
  const canWrite = useCanWriteSensitive()
  const isRoot = useIsRoot()
  const domains = useMailDomains(up).data ?? []
  const teams = useQuery(teamListQueryOptions()).data
  const queueQ = useMailQueue(up)
  const queued =
    queueQ.isError && isHiddenQueueError(queueQ.error)
      ? 0
      : (queueQ.data?.queued ?? 0)
  const active = tab ?? local
  // Palette deep links (?add, ?queue) act once, then clear so they can fire again.
  const [seen, setSeen] = useState({ add: !!add, queue: !!queue })
  if (!!add !== seen.add || !!queue !== seen.queue) {
    setSeen({ add: !!add, queue: !!queue })
    if (add) setAdding(true)
    if (queue) setQueueOpen(true)
  }
  useEffect(() => {
    if (add || queue) onHandled?.()
  }, [add, queue, onHandled])
  const setTab = useCallback(
    (next: MailTab) => {
      setLocal(next)
      storeMailTab(next)
      onTab?.(next)
    },
    [onTab],
  )
  const startAdd = () => {
    setAdding(true)
    setTab('mailboxes')
  }
  const tlsLabel = status
    ? t(`next.tls.${status.tls}`, { defaultValue: t('next.tls.unknown') })
    : ''
  const stateKey = !status?.installed
    ? 'shell.notInstalled'
    : up
      ? 'shell.running'
      : status.container_state === 'running'
        ? 'shell.waiting'
        : 'shell.stopped'

  return (
    <PageTemplate
      eyebrow={t('eyebrow')}
      icon={<MailGlassIcon size="1.6em" />}
      title={t('title')}
      description={t('description')}
      actions={
        status?.installed ? (
          <div className="flex max-w-full min-w-0 flex-wrap items-center justify-end gap-2">
            <MailWebmailButton app={status.app} />
            {up ? (
              <span title={canWrite ? undefined : t('mailboxes.needsWrite')}>
                <Button
                  size="sm"
                  className="gap-1.5"
                  disabled={!canWrite}
                  onClick={startAdd}
                >
                  <PlusGlassIcon size="1.1em" />
                  {t('shell.addMailbox')}
                </Button>
              </span>
            ) : null}
          </div>
        ) : (
          <Link
            to="/templates"
            search={{ deploy: 'mail-mox' }}
            className={cn(buttonVariants({ variant: 'outline' }), 'gap-1.5')}
            title={t('install.body')}
          >
            <RocketGlassIcon size="1.2em" />
            {t('install.action')}
          </Link>
        )
      }
    >
      <div className="space-y-4">
        {status?.demo ? <MailDemoBadge /> : null}
        {status?.maintenance_hold ? (
          <MailHoldBanner hold={status.maintenance_hold} />
        ) : null}
        <div
          role="status"
          aria-label={t('shell.statusLabel')}
          className="flex min-h-8 flex-wrap items-center gap-2"
        >
          {status ? (
            <>
              <Badge variant={up ? 'success' : 'warning'}>{t(stateKey)}</Badge>
              {status.installed ? (
                <Badge
                  variant={status.tls === 'self-signed' ? 'warning' : 'muted'}
                  title={
                    status.tls === 'self-signed'
                      ? t('next.tlsSelfSigned')
                      : undefined
                  }
                >
                  {tlsLabel}
                </Badge>
              ) : null}
              {queued > 0 ? (
                <button
                  type="button"
                  onClick={() => setQueueOpen(true)}
                  className="rounded-full border border-border px-2.5 py-0.5 text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
                >
                  {t('shell.queueChip', { count: queued })}
                </button>
              ) : null}
              {up ? (
                <Button
                  size="icon-sm"
                  variant="ghost"
                  className="ml-auto"
                  aria-label={t('shell.settings')}
                  title={t('shell.settings')}
                  onClick={() => setSettings(true)}
                >
                  <SettingsGlassIcon size="1.3em" />
                </Button>
              ) : null}
            </>
          ) : (
            <Skeleton aria-label={t('shell.skeleton')} className="h-6 w-48" />
          )}
        </div>
        <Tabs
          value={active}
          onValueChange={(v) => setTab(parseMailTab(v) ?? 'overview')}
        >
          <TabsList variant="line" aria-label={t('tabs.label')}>
            {MAIL_TABS.map((k) => (
              <TabsTrigger key={k} value={k}>
                {t(`tabs.${k}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        {active === 'overview' && status ? (
          <MailOverview
            status={status}
            report={report}
            domain={domain}
            onReport={setReport}
            onPreview={() => {
              setPreviewToken((n) => n + 1)
              setTab('domains')
            }}
            onTab={setTab}
          />
        ) : null}
        {active === 'mailboxes' ? (
          up ? (
            <MailMailboxes
              domains={domains}
              adding={adding}
              onAdding={setAdding}
            />
          ) : (
            <p className="text-sm text-muted-foreground">{t('mbx.notUp')}</p>
          )
        ) : null}
        {active === 'domains' ? (
          <div className="space-y-3">
            {up ? (
              <div className="flex flex-wrap items-center gap-2">
                {domains.map((d) => (
                  <Badge key={d.domain} variant="muted">
                    {d.domain}
                  </Badge>
                ))}
                <Popover>
                  <PopoverTrigger
                    render={
                      <Button size="xs" variant="ghost" className="gap-1" />
                    }
                  >
                    <PlusGlassIcon size="1.1em" />
                    {t('mbx.addDomain')}
                  </PopoverTrigger>
                  <PopoverContent align="start" className="w-auto">
                    <MailDomainClaim
                      domains={domains}
                      isRoot={isRoot}
                      teams={teams ?? []}
                    />
                  </PopoverContent>
                </Popover>
              </div>
            ) : null}
            <MailDnsPanel
              report={report}
              onReport={setReport}
              onDomain={setDomain}
              defaults={status}
              previewToken={previewToken}
            />
          </div>
        ) : null}
        {active === 'backups' ? (
          up && status?.app ? (
            <MailBackupLine
              app={status.app}
              onStorage={() => setSettings(true)}
            />
          ) : (
            <p className="text-sm text-muted-foreground">{t('mbx.notUp')}</p>
          )
        ) : null}
      </div>
      <MailSettingsSheet
        open={settings}
        onOpenChange={setSettings}
        domains={domains}
        app={status?.app}
      />
      <MailQueueSheet open={queueOpen} onOpenChange={setQueueOpen} />
    </PageTemplate>
  )
}
