import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsLeftRightIcon,
  CheckCircleIcon,
  CircleDashedIcon,
  PauseCircleIcon,
  PaperPlaneTiltIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import {
  providerByRole,
  useDeleteEmailProvider,
  useEmailProviders,
  usePromoteEmailProvider,
  useUpdateEmailProvider,
  type EmailProvider,
  type EmailProviderRole,
} from '../../queries/emailProviders'
import { useSendTestEmail } from '../../queries/emailSettings'
import { EmailProviderPanel } from './EmailProviderPanel'
import { EmailProviderPicker } from './EmailProviderPicker'
import type { EmailProviderPreset } from './emailProviderPresets'
import { EmailTestOutcome } from './EmailTestOutcome'
import { ProviderMark } from './ProviderMark'
import { providerFromAddress, toUnchangedPayload } from './emailProviderForm'
import { providerById } from './emailProviders'
import { providerHealth, type ProviderHealth } from './providerHealth'
import { CARD_FLOAT, PRESS } from './emailStyles'

const HEALTH_ICON = {
  healthy: CheckCircleIcon,
  failing: WarningCircleIcon,
  paused: PauseCircleIcon,
  untested: CircleDashedIcon,
} as const

const HEALTH_VARIANT = {
  healthy: 'success',
  failing: 'destructive',
  paused: 'muted',
  untested: 'muted',
} as const satisfies Record<ProviderHealth, string>

export function ProviderStatusChip({ provider }: { provider: EmailProvider }) {
  const { t } = useTranslation('settings')
  const health = providerHealth(provider)
  const Icon = HEALTH_ICON[health]
  return (
    <Badge variant={HEALTH_VARIANT[health]}>
      <Icon className="size-3" aria-hidden="true" />
      {t(`emailProviders.health.${health}`)}
    </Badge>
  )
}

interface RowProps {
  provider: EmailProvider
  role: EmailProviderRole
  sharedFrom: string
  hasBackup: boolean
  editing: boolean
  sesRegions: string[]
  onEdit: () => void
  onClose: () => void
}

function ProviderRow({
  provider,
  role,
  sharedFrom,
  hasBackup,
  editing,
  sesRegions,
  onEdit,
  onClose,
}: Readonly<RowProps>) {
  const { t } = useTranslation('settings')
  const update = useUpdateEmailProvider()
  const promote = usePromoteEmailProvider()
  const remove = useDeleteEmailProvider()
  const test = useSendTestEmail()
  const [confirmRemove, setConfirmRemove] = useState(false)
  const id = provider.id ?? ''
  const entry = providerById(provider.backend)
  const backendLabel = t(`emailDelivery.providers.${provider.backend}`)
  const name = provider.name || backendLabel
  const from = providerFromAddress(provider, sharedFrom)
  const enabled = provider.enabled !== false

  return (
    <li className="space-y-2">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border bg-background/40 px-3 py-2">
        <div className="flex min-w-48 flex-1 items-center gap-2">
          <ProviderMark
            logoId={entry?.logoId}
            label={backendLabel}
            generic={provider.backend === 'smtp'}
          />
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              <span className="truncate text-sm font-medium">{name}</span>
              <Badge variant={role === 'primary' ? 'accent' : 'outline'}>
                {t(`emailProviders.roles.${role}`)}
              </Badge>
            </div>
            <div className="truncate text-xs text-muted-foreground">
              {from || t('emailProviders.row.noFrom')}
            </div>
          </div>
        </div>
        <div className="w-28 text-xs text-muted-foreground">
          {provider.last_ok_at ? (
            <span title={formatAbsoluteTime(provider.last_ok_at)}>
              {t('emailProviders.row.lastOk', {
                when: formatRelativeTime(provider.last_ok_at),
              })}
            </span>
          ) : (
            t('emailProviders.row.neverOk')
          )}
        </div>
        <ProviderStatusChip provider={provider} />
        <Switch
          size="sm"
          checked={enabled}
          disabled={update.isPending || !id}
          aria-label={t('emailProviders.row.enabledLabel', { name })}
          onCheckedChange={(next) => {
            update.mutate(toUnchangedPayload(provider, next))
          }}
        />
        <div className="flex flex-wrap items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className={PRESS}
            aria-expanded={editing}
            aria-label={t('emailProviders.row.editLabel', { name })}
            onClick={editing ? onClose : onEdit}
          >
            <PencilSimpleIcon aria-hidden="true" />
            {t('emailProviders.row.edit')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className={PRESS}
            disabled={test.isPending}
            aria-label={t('emailProviders.row.testLabel', { name })}
            onClick={() => {
              test.mutate({ to: '', providerId: id })
            }}
          >
            <PaperPlaneTiltIcon aria-hidden="true" />
            {test.isPending
              ? t('emailDelivery.test.sending')
              : t('emailProviders.row.test')}
          </Button>
          {role === 'backup' ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className={PRESS}
              disabled={promote.isPending}
              aria-label={t('emailProviders.row.promoteLabel', { name })}
              onClick={() => {
                promote.mutate(id, {
                  onSuccess: () => {
                    toast.add({
                      title: t('emailProviders.row.promoted', { name }),
                      type: 'success',
                    })
                  },
                })
              }}
            >
              <ArrowsLeftRightIcon aria-hidden="true" />
              {t('emailProviders.row.promote')}
            </Button>
          ) : null}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className={cn('text-destructive', PRESS)}
            aria-label={t('emailProviders.row.removeLabel', { name })}
            onClick={() => {
              setConfirmRemove(true)
            }}
          >
            <TrashIcon aria-hidden="true" />
            {t('emailProviders.row.remove')}
          </Button>
        </div>
      </div>
      {provider.last_error_kind ? (
        <p className="px-3 text-xs text-tone-danger">
          {t('emailProviders.row.lastError', {
            kind: t(
              `emailDelivery.failure.${provider.last_error_kind as 'other'}.title`,
            ),
          })}
        </p>
      ) : null}
      {update.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{update.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {promote.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{promote.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <EmailTestOutcome
        data={test.data}
        error={test.error}
        backend={provider.backend}
      />
      {editing ? (
        <EmailProviderPanel
          provider={provider}
          role={role}
          sharedFrom={sharedFrom}
          sesRegions={sesRegions}
          onClose={onClose}
        />
      ) : null}
      <ConfirmDialog
        open={confirmRemove}
        onOpenChange={(open) => {
          setConfirmRemove(open)
          if (!open) remove.reset()
        }}
        title={t('emailProviders.remove.title', { name })}
        description={
          role === 'primary' && hasBackup
            ? t('emailProviders.remove.primaryWithBackup')
            : t('emailProviders.remove.description')
        }
        consequences={[
          t('emailProviders.remove.consequenceCredentials'),
          role === 'backup'
            ? t('emailProviders.remove.consequenceNoFailover')
            : t('emailProviders.remove.consequenceNoMail'),
        ]}
        confirmLabel={t('emailProviders.remove.confirm')}
        pendingLabel={t('emailProviders.remove.removing')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : null}
        onConfirm={() => {
          remove.mutate(id, {
            onSuccess: () => {
              setConfirmRemove(false)
              toast.add({
                title: t('emailProviders.remove.done', { name }),
                type: 'success',
              })
            },
          })
        }}
      />
    </li>
  )
}

function AddSlot({
  role,
  onAdd,
}: {
  role: EmailProviderRole
  onAdd: () => void
}) {
  const { t } = useTranslation('settings')
  return (
    <li className="flex flex-wrap items-center gap-3 rounded-lg border border-dashed border-border px-3 py-2">
      <div className="min-w-48 flex-1">
        <div className="text-sm font-medium">
          {t(`emailProviders.add.${role}.title`)}
        </div>
        <p className="text-xs text-muted-foreground">
          {t(`emailProviders.add.${role}.body`)}
        </p>
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={PRESS}
        onClick={onAdd}
      >
        <PlusIcon aria-hidden="true" />
        {t(`emailProviders.add.${role}.action`)}
      </Button>
    </li>
  )
}

type Flow =
  | { kind: 'edit'; id: string }
  | { kind: 'pick'; role: EmailProviderRole }
  | { kind: 'add'; role: EmailProviderRole; preset: EmailProviderPreset }

/** EmailProvidersCard lists the primary and backup senders with their health and per-row actions. */
export function EmailProvidersCard({
  sharedFrom,
  sesRegions,
}: Readonly<{ sharedFrom: string; sesRegions: string[] }>) {
  const { t } = useTranslation('settings')
  const { data, isPending, isError, error, refetch } = useEmailProviders()
  const [flow, setFlow] = useState<Flow | null>(null)
  const close = () => {
    setFlow(null)
  }
  const primary = providerByRole(data, 'primary')
  const backup = providerByRole(data, 'backup')
  const editingId = flow?.kind === 'edit' ? flow.id : null

  const slot = (role: EmailProviderRole) => {
    if (flow?.kind === 'add' && flow.role === role) {
      return (
        <li>
          <EmailProviderPanel
            role={role}
            preset={flow.preset}
            sharedFrom={sharedFrom}
            sesRegions={sesRegions}
            onClose={close}
          />
        </li>
      )
    }
    // With no provider at all the templates are the empty state, so they show without a click.
    if (
      (flow?.kind === 'pick' && flow.role === role) ||
      (role === 'primary' && !primary)
    ) {
      return (
        <li>
          <EmailProviderPicker
            onPick={(preset) => {
              setFlow({ kind: 'add', role, preset })
            }}
            onCancel={role === 'backup' ? close : undefined}
          />
        </li>
      )
    }
    return (
      <AddSlot
        role={role}
        onAdd={() => {
          setFlow({ kind: 'pick', role })
        }}
      />
    )
  }

  return (
    <Card size="sm" className={CARD_FLOAT}>
      <CardContent className="space-y-2">
        <div>
          <h2 className="text-sm font-semibold">{t('emailProviders.title')}</h2>
          <p className="text-xs text-muted-foreground">
            {t('emailProviders.description')}
          </p>
        </div>
        {isPending ? (
          <div className="space-y-2" aria-hidden="true">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        ) : isError ? (
          <Alert variant="destructive">
            <AlertDescription className="flex flex-wrap items-center gap-2">
              <span className="min-w-0 flex-1">
                {t('emailProviders.loadError', { message: error.message })}
              </span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className={PRESS}
                onClick={() => {
                  void refetch()
                }}
              >
                {t('emailProviders.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <ul className="space-y-2">
            {primary ? (
              <ProviderRow
                provider={primary}
                role="primary"
                sharedFrom={sharedFrom}
                hasBackup={Boolean(backup)}
                editing={editingId === primary.id}
                sesRegions={sesRegions}
                onEdit={() => {
                  setFlow({ kind: 'edit', id: primary.id ?? '' })
                }}
                onClose={close}
              />
            ) : (
              slot('primary')
            )}
            {backup ? (
              <ProviderRow
                provider={backup}
                role="backup"
                sharedFrom={sharedFrom}
                hasBackup
                editing={editingId === backup.id}
                sesRegions={sesRegions}
                onEdit={() => {
                  setFlow({ kind: 'edit', id: backup.id ?? '' })
                }}
                onClose={close}
              />
            ) : primary ? (
              slot('backup')
            ) : null}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
