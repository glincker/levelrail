import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  PencilGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import {
  type PasskeyResource,
  useDeletePasskey,
  useRenamePasskey,
} from '@/queries/passkeys'
import { classifyPasskeyError } from '@/lib/passkeyErrors'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { PasskeyProviderIcon } from './PasskeyProviderIcon'

const KNOWN_TRANSPORTS = [
  'usb',
  'nfc',
  'ble',
  'internal',
  'hybrid',
  'smart-card',
] as const

type KnownTransport = (typeof KNOWN_TRANSPORTS)[number]

function isKnownTransport(x: string): x is KnownTransport {
  return (KNOWN_TRANSPORTS as readonly string[]).includes(x)
}

function When({ iso }: { iso: string }) {
  return (
    <time dateTime={iso} title={formatAbsoluteTime(iso)}>
      {formatRelativeTime(iso)}
    </time>
  )
}

export function PasskeyRow({
  passkey,
  isLast,
  onAddAnother,
}: {
  passkey: PasskeyResource
  isLast: boolean
  onAddAnother: () => void
}) {
  const { t } = useTranslation('passkeys')
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(passkey.label)
  const [removeOpen, setRemoveOpen] = useState(false)
  const rename = useRenamePasskey()
  const del = useDeletePasskey()

  function submitRename(e: FormEvent) {
    e.preventDefault()
    const next = draft.trim()
    if (next === '' || next === passkey.label) {
      setEditing(false)
      return
    }
    rename.mutate(
      { id: passkey.id, label: next },
      { onSuccess: () => setEditing(false) },
    )
  }

  const blocked =
    del.isError && classifyPasskeyError(del.error) === 'lastMethod'
  const providerName = passkey.provider_name ?? t('row.provider')
  const transports = (passkey.transports ?? []).filter(isKnownTransport)

  return (
    <li className="flex flex-wrap items-start justify-between gap-3 py-3 text-sm">
      <div className="flex min-w-0 items-start gap-3">
        <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border border-border bg-card">
          <PasskeyProviderIcon providerKey={passkey.provider_key} />
        </span>
        <div className="min-w-0 space-y-1">
          {editing ? (
            <form onSubmit={submitRename} className="flex items-center gap-1">
              <Input
                value={draft}
                maxLength={64}
                autoFocus
                aria-label={t('row.renameLabel')}
                onChange={(e) => {
                  setDraft(e.target.value)
                }}
                className="h-8 w-56"
              />
              <Button
                type="submit"
                size="icon-sm"
                variant="ghost"
                disabled={rename.isPending}
                aria-label={t('row.save')}
              >
                <CheckGlassIcon size="1.4em" />
              </Button>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                aria-label={t('row.cancel')}
                onClick={() => {
                  setEditing(false)
                  setDraft(passkey.label)
                  rename.reset()
                }}
              >
                <XGlassIcon size="1.4em" />
              </Button>
            </form>
          ) : (
            <p className="flex items-center gap-1 font-medium text-foreground">
              <span className="truncate">{passkey.label}</span>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                aria-label={t('row.rename', { name: passkey.label })}
                onClick={() => {
                  setDraft(passkey.label)
                  setEditing(true)
                }}
              >
                <PencilGlassIcon size="1.4em" />
              </Button>
            </p>
          )}
          {rename.isError ? (
            <p className="text-xs text-destructive" role="alert">
              {t('row.renameFailed')}
            </p>
          ) : null}
          <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <span>{providerName}</span>
            {passkey.synced === undefined ? null : (
              <Tooltip>
                <TooltipTrigger
                  render={<Badge variant={passkey.synced ? 'info' : 'muted'} />}
                >
                  {passkey.synced ? t('row.synced') : t('row.deviceBound')}
                </TooltipTrigger>
                <TooltipContent>
                  {passkey.synced
                    ? t('row.syncedHint')
                    : t('row.deviceBoundHint')}
                </TooltipContent>
              </Tooltip>
            )}
            {transports.map((x) => (
              <Badge key={x} variant="outline">
                {t(`row.transport.${x}`)}
              </Badge>
            ))}
          </div>
          <p className="text-xs text-muted-foreground">
            {t('row.added')} <When iso={passkey.created_at} />
            {' · '}
            {passkey.last_used_at ? (
              <>
                {t('row.lastUsed')} <When iso={passkey.last_used_at} />
              </>
            ) : (
              t('row.neverUsed')
            )}
            {' · '}
            {t('row.signIns', { count: passkey.sign_count })}
          </p>
        </div>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-label={t('row.remove', { name: passkey.label })}
        onClick={() => {
          del.reset()
          setRemoveOpen(true)
        }}
      >
        <TrashGlassIcon size="1.4em" />
      </Button>
      <ConfirmDialog
        open={removeOpen}
        onOpenChange={(next) => {
          setRemoveOpen(next)
          if (!next) del.reset()
        }}
        title={t('remove.title')}
        description={t('remove.description', { name: passkey.label })}
        confirmLabel={t('remove.confirm')}
        pendingLabel={t('remove.pending')}
        tone="destructive"
        pending={del.isPending}
        error={del.isError && !blocked ? t('remove.blockedFailed') : null}
        onConfirm={() => {
          del.mutate(passkey.id, { onSuccess: () => setRemoveOpen(false) })
        }}
      >
        {blocked ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription className="space-y-2">
              <p className="font-medium">{t('remove.blockedTitle')}</p>
              <p>{t('remove.blockedBody')}</p>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setRemoveOpen(false)
                  onAddAnother()
                }}
              >
                {t('remove.blockedAddPasskey')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : isLast ? (
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription>{t('remove.lastWarning')}</AlertDescription>
          </Alert>
        ) : null}
      </ConfirmDialog>
    </li>
  )
}
