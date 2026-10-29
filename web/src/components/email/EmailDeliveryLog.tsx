import { useDeferredValue, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  FileSearchGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { LoadMore } from '@/components/ui/load-more'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import {
  useEmailDeliveries,
  type EmailDelivery,
  type EmailDeliveryFilters,
} from '../../queries/emailSettings'
import { EMAIL_PROVIDERS } from './emailProviders'

const ALL = 'all'

function StatusPill({ status }: { status: EmailDelivery['status'] }) {
  const { t } = useTranslation('settings')
  const variant =
    status === 'sent'
      ? 'success'
      : status === 'failed'
        ? 'destructive'
        : 'warning'
  const Icon = status === 'sent' ? CircleCheckGlassIcon : CircleAlertGlassIcon
  return (
    <Badge variant={variant}>
      <Icon className="size-3" aria-hidden="true" />
      {t(`emailDelivery.log.statuses.${status}`)}
    </Badge>
  )
}

function DetailRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  )
}

function DeliveryDrawer({
  delivery,
  onClose,
}: {
  delivery: EmailDelivery | null
  onClose: () => void
}) {
  const { t } = useTranslation('settings')
  return (
    <Sheet
      open={delivery !== null}
      onOpenChange={(o) => (o ? null : onClose())}
    >
      <SheetContent className="overflow-y-auto sm:max-w-lg">
        {delivery ? (
          <>
            <SheetHeader>
              <SheetTitle>{t('emailDelivery.log.detailTitle')}</SheetTitle>
              <SheetDescription>
                {formatAbsoluteTime(delivery.created_at)}
              </SheetDescription>
            </SheetHeader>
            <dl className="space-y-3 px-4 pb-4">
              <DetailRow label={t('emailDelivery.log.columns.status')}>
                <StatusPill status={delivery.status} />
              </DetailRow>
              <DetailRow label={t('emailDelivery.log.columns.to')}>
                {delivery.to}
              </DetailRow>
              <DetailRow label={t('emailDelivery.log.columns.provider')}>
                {delivery.provider}
              </DetailRow>
              <DetailRow label={t('emailDelivery.log.columns.kind')}>
                {delivery.kind || t('emailDelivery.log.noKind')}
              </DetailRow>
              <DetailRow label={t('emailDelivery.log.attempts')}>
                {delivery.attempts}
              </DetailRow>
              {delivery.message_id ? (
                <DetailRow label={t('emailDelivery.log.messageId')}>
                  <span className="font-mono text-xs">
                    {delivery.message_id}
                  </span>
                </DetailRow>
              ) : null}
              {delivery.error_kind ? (
                <DetailRow label={t('emailDelivery.log.failureKind')}>
                  {t(
                    `emailDelivery.failure.${delivery.error_kind as 'other'}.title`,
                  )}
                  <p className="text-muted-foreground">
                    {t(
                      `emailDelivery.failure.${delivery.error_kind as 'other'}.fix`,
                    )}
                  </p>
                </DetailRow>
              ) : null}
              {delivery.error ? (
                <DetailRow label={t('emailDelivery.log.error')}>
                  <span className="font-mono text-xs">{delivery.error}</span>
                </DetailRow>
              ) : null}
            </dl>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}

/** EmailDeliveryLog is the searchable delivery list with status and provider filters and a detail drawer. */
export function EmailDeliveryLog() {
  const { t } = useTranslation('settings')
  const [status, setStatus] = useState<EmailDeliveryFilters['status']>('')
  const [provider, setProvider] = useState('')
  const [search, setSearch] = useState('')
  const q = useDeferredValue(search.trim())
  const [selected, setSelected] = useState<EmailDelivery | null>(null)

  const query = useEmailDeliveries({ status, provider, q })
  const rows = useMemo(
    () => query.data?.pages.flatMap((p) => p.deliveries) ?? [],
    [query.data],
  )

  const columns: ResponsiveColumn<EmailDelivery>[] = [
    {
      id: 'time',
      header: t('emailDelivery.log.columns.time'),
      primary: true,
      cell: (d) => (
        <span title={formatAbsoluteTime(d.created_at)}>
          {formatRelativeTime(d.created_at)}
        </span>
      ),
    },
    {
      id: 'status',
      header: t('emailDelivery.log.columns.status'),
      cell: (d) => <StatusPill status={d.status} />,
    },
    {
      id: 'to',
      header: t('emailDelivery.log.columns.to'),
      cell: (d) => <span className="font-mono text-xs">{d.to}</span>,
    },
    {
      id: 'provider',
      header: t('emailDelivery.log.columns.provider'),
      priority: 2,
      cell: (d) => d.provider,
    },
    {
      id: 'kind',
      header: t('emailDelivery.log.columns.kind'),
      priority: 3,
      cell: (d) => d.kind || t('emailDelivery.log.noKind'),
    },
  ]

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('emailDelivery.log.searchPlaceholder')}
          aria-label={t('emailDelivery.log.searchPlaceholder')}
          className="sm:max-w-xs"
        />
        <Select
          value={status === '' ? ALL : status}
          onValueChange={(v) =>
            setStatus(v === ALL ? '' : (v as EmailDeliveryFilters['status']))
          }
        >
          <SelectTrigger
            aria-label={t('emailDelivery.log.columns.status')}
            className="sm:w-40"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>
              {t('emailDelivery.log.allStatuses')}
            </SelectItem>
            {(['sent', 'failed', 'bounced'] as const).map((s) => (
              <SelectItem key={s} value={s}>
                {t(`emailDelivery.log.statuses.${s}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={provider === '' ? ALL : provider}
          onValueChange={(v) => setProvider(v === ALL ? '' : (v ?? ''))}
        >
          <SelectTrigger
            aria-label={t('emailDelivery.log.columns.provider')}
            className="sm:w-48"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>
              {t('emailDelivery.log.allProviders')}
            </SelectItem>
            {EMAIL_PROVIDERS.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {t(`emailDelivery.providers.${p.labelKey}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {query.isError ? (
        <Alert variant="destructive">
          <AlertDescription className="flex items-center gap-2">
            {query.error.message}
            <Button
              size="sm"
              variant="outline"
              onClick={() => void query.refetch()}
            >
              {t('emailDelivery.log.retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}

      {!query.isPending && rows.length === 0 && !query.isError ? (
        <EmptyState
          icon={<FileSearchGlassIcon className="size-5.5" />}
          title={t('emailDelivery.log.emptyTitle')}
          description={t('emailDelivery.log.emptyBody')}
        />
      ) : (
        <ResponsiveTable
          rows={rows}
          columns={columns}
          getRowId={(d) => d.id}
          ariaLabel={t('emailDelivery.log.title')}
          onRowClick={setSelected}
        />
      )}
      <LoadMore
        loaded={rows.length}
        total={query.hasNextPage ? rows.length + 1 : rows.length}
        loading={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
      />
      <DeliveryDrawer delivery={selected} onClose={() => setSelected(null)} />
    </div>
  )
}
