import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BellSlashIcon, CheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { formatAge } from '../lib/format'
import { certNoticeFromItem } from './certNotice'
import {
  useInbox,
  useInboxPrefs,
  useMarkInboxRead,
  useSaveInboxPrefs,
  type InboxItem,
  type InboxPrefs,
} from '../queries/inbox'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'

const SEVERITIES: InboxPrefs['min_severity'][] = ['info', 'warning', 'critical']
const HOURS = Array.from({ length: 24 }, (_, h) => h)

const SEVERITY_DOT: Record<string, string> = {
  critical: 'bg-destructive',
  warning: 'bg-amber-500',
  info: 'bg-sky-500',
}

export function AlertInboxPanel() {
  const { t } = useTranslation('inbox')
  const [unreadOnly, setUnreadOnly] = useState(false)
  const inbox = useInbox(unreadOnly)
  const mark = useMarkInboxRead()
  const items = inbox.data ?? []
  const unread = items.filter((i) => !i.read).length

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t('description')}</p>
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-2 text-sm">
            <Switch
              size="sm"
              checked={unreadOnly}
              onCheckedChange={setUnreadOnly}
            />
            {t('unreadOnly')}
          </label>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={unread === 0 || mark.isPending}
            onClick={() => {
              mark.mutate(
                { all: true },
                {
                  onSuccess: () => {
                    toast.add({ title: t('markedAll'), type: 'success' })
                  },
                },
              )
            }}
          >
            <CheckIcon />
            {t('markAllRead')}
          </Button>
        </div>
      </div>

      {inbox.isPending ? (
        <ListSkeleton rows={4} />
      ) : inbox.isError ? (
        <EmptyState
          compact
          icon={<BellSlashIcon className="size-5" />}
          title={t('loadError')}
          description={inbox.error.message}
          action={
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void inbox.refetch()}
            >
              {t('retry')}
            </Button>
          }
        />
      ) : items.length === 0 ? (
        <EmptyState
          compact
          icon={<CheckIcon className="size-5" />}
          title={t('empty.title')}
          description={t('empty.body')}
        />
      ) : (
        <ul className="divide-y rounded-lg border">
          {items.map((i) => (
            <InboxRow
              key={i.id}
              item={i}
              onRead={() => {
                mark.mutate({ ids: [i.id] })
              }}
            />
          ))}
        </ul>
      )}

      <InboxPrefsForm />
    </div>
  )
}

const CertExpiryInboxCard = lazy(() =>
  import('./CertExpiryInboxCard').then((m) => ({
    default: m.CertExpiryInboxCard,
  })),
)

function InboxRow({ item, onRead }: { item: InboxItem; onRead: () => void }) {
  const { t } = useTranslation('inbox')
  const target = item.app ?? item.node ?? ''
  const certNotice =
    item.rule_kind === 'cert_expiry' ? certNoticeFromItem(item) : null
  return (
    <li className="flex items-start gap-3 px-3 py-2.5">
      <span
        aria-hidden="true"
        className={cn(
          'mt-1.5 size-2 shrink-0 rounded-full',
          item.read
            ? 'bg-muted-foreground/30'
            : (SEVERITY_DOT[item.severity ?? 'warning'] ?? 'bg-amber-500'),
        )}
      />
      <div className="min-w-0 flex-1">
        <p
          className={cn(
            'truncate text-sm',
            item.read ? 'text-muted-foreground' : 'font-medium',
          )}
        >
          {item.rule_name} {t(`event.${item.event}`, item.event)}
          {target ? ` (${target})` : ''}
        </p>
        <p className="text-xs text-muted-foreground">
          {formatAge(item.at)}
          {item.detail && !certNotice ? ` - ${item.detail}` : ''}
        </p>
        {certNotice && item.event === 'fired' ? (
          <div className="mt-2">
            <Suspense fallback={null}>
              <CertExpiryInboxCard notice={certNotice} />
            </Suspense>
          </div>
        ) : null}
      </div>
      <Badge variant="outline">
        {t(`severity.${item.severity ?? 'warning'}`)}
      </Badge>
      {item.read ? null : (
        <Button type="button" size="sm" variant="ghost" onClick={onRead}>
          {t('markRead')}
        </Button>
      )}
    </li>
  )
}

function InboxPrefsForm() {
  const prefs = useInboxPrefs()
  if (!prefs.data) return null
  return <PrefsFields key={JSON.stringify(prefs.data)} initial={prefs.data} />
}

function PrefsFields({ initial }: { initial: InboxPrefs }) {
  const { t } = useTranslation('inbox')
  const save = useSaveInboxPrefs()
  const channels = useNotificationChannelsOptional()
  const [draft, setDraft] = useState<InboxPrefs>(initial)
  const [muted, setMuted] = useState(initial.muted_apps.join(', '))
  const list = channels.data ?? []
  const needsChannel = draft.digest_enabled && draft.digest_channel_id === ''

  return (
    <form
      className="space-y-4 rounded-lg border p-4"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(
          {
            ...draft,
            muted_apps: muted
              .split(',')
              .map((s) => s.trim())
              .filter(Boolean),
          },
          {
            onSuccess: () => {
              toast.add({ title: t('prefs.saved'), type: 'success' })
            },
            onError: (err) => {
              toast.add({
                title: t('prefs.saveError'),
                description: err.message,
                type: 'error',
              })
            },
          },
        )
      }}
    >
      <h3 className="text-sm font-medium">{t('prefs.title')}</h3>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel>{t('prefs.minSeverity')}</FieldLabel>
          <Select
            value={draft.min_severity}
            onValueChange={(v) => {
              setDraft({
                ...draft,
                min_severity: v as InboxPrefs['min_severity'],
              })
            }}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SEVERITIES.map((s) => (
                <SelectItem key={s} value={s}>
                  {t(`severity.${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel>{t('prefs.mutedApps')}</FieldLabel>
          <Input
            value={muted}
            onChange={(e) => {
              setMuted(e.target.value)
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('prefs.mutedAppsHint')}
          </p>
        </Field>
      </div>
      <div className="space-y-3">
        <label className="flex items-center gap-2 text-sm font-medium">
          <Switch
            checked={draft.digest_enabled}
            onCheckedChange={(v) => {
              setDraft({ ...draft, digest_enabled: v })
            }}
          />
          {t('prefs.digest')}
        </label>
        <p className="text-xs text-muted-foreground">{t('prefs.digestHint')}</p>
        {draft.digest_enabled ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel>{t('prefs.digestHour')}</FieldLabel>
              <Select
                value={String(draft.digest_hour)}
                onValueChange={(v) => {
                  setDraft({ ...draft, digest_hour: Number(v) })
                }}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {HOURS.map((h) => (
                    <SelectItem key={h} value={String(h)}>
                      {String(h).padStart(2, '0')}:00
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel>{t('prefs.digestChannel')}</FieldLabel>
              <Select
                value={draft.digest_channel_id}
                onValueChange={(v) => {
                  setDraft({ ...draft, digest_channel_id: v ?? '' })
                }}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder={t('prefs.noChannel')} />
                </SelectTrigger>
                <SelectContent>
                  {list.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </div>
        ) : null}
        {needsChannel ? (
          <p role="status" className="text-xs text-destructive">
            {t('prefs.digestNeedsChannel')}
          </p>
        ) : null}
      </div>
      <Button type="submit" size="sm" disabled={save.isPending || needsChannel}>
        {t('prefs.save')}
      </Button>
    </form>
  )
}
