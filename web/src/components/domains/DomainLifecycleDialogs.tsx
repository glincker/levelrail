import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import { appListQueryOptions } from '../../queries/apps'
import { useEditAppDomains } from '../../queries/appEnvironmentDomains'
import {
  useAttachParkedDomain,
  useDeleteParkedDomain,
  useMoveDomain,
  useParkDomain,
} from '../../queries/parkedDomains'
import { humanizeSettingKind, moveTargets } from '../../lib/domainLifecycle'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function errorText(e: unknown): string | null {
  return e instanceof Error ? e.message : null
}

// Parking snapshots the per-domain settings; attach restores them unless discarded.
export function ParkDomainDialog({
  app,
  domain,
  open,
  onOpenChange,
}: Props & { app: string; domain: string }) {
  const { t } = useTranslation('domains')
  const park = useParkDomain()
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('lifecycle.park.title', { domain })}
      description={t('lifecycle.park.body', { app })}
      consequences={[
        t('lifecycle.park.savesSettings'),
        t('lifecycle.park.keepsDnsCert'),
      ]}
      confirmLabel={t('lifecycle.park.confirm')}
      pendingLabel={t('lifecycle.pending')}
      focusCancel
      pending={park.isPending}
      error={errorText(park.error)}
      onConfirm={() => {
        park.mutate(
          { app, domain },
          {
            onSuccess: () => {
              toast.add({
                title: t('lifecycle.park.done', { domain }),
                type: 'success',
              })
              onOpenChange(false)
            },
          },
        )
      }}
    />
  )
}

// Move keeps WAF, own certificate and basic auth; the first press runs the
// collision check, the second (when asked) confirms.
export function MoveDomainDialog({
  from,
  domain,
  open,
  onOpenChange,
}: Props & { from: string; domain: string }) {
  const { t } = useTranslation('domains')
  const { data: apps = [] } = useQuery(appListQueryOptions())
  const targets = moveTargets(apps, from)
  const [to, setTo] = useState('')
  const [warning, setWarning] = useState<string | null>(null)
  const move = useMoveDomain()
  const target = to || targets[0] || ''
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setWarning(null)
        onOpenChange(next)
      }}
      title={t('lifecycle.move.title', { domain })}
      description={t('lifecycle.move.body', { app: from })}
      consequences={[t('lifecycle.move.keepsSettings')]}
      confirmLabel={
        warning ? t('lifecycle.move.confirm') : t('lifecycle.move.check')
      }
      pendingLabel={t('lifecycle.pending')}
      confirmDisabled={target === ''}
      pending={move.isPending}
      error={errorText(move.error)}
      onConfirm={() => {
        move.mutate(
          { domain, from, to: target, confirm: warning !== null },
          {
            onSuccess: (result) => {
              if (result.applied) {
                toast.add({
                  title: t('lifecycle.move.done', { domain, app: target }),
                  type: 'success',
                })
                setWarning(null)
                onOpenChange(false)
              } else {
                setWarning(result.message ?? t('lifecycle.move.needsConfirm'))
              }
            },
          },
        )
      }}
    >
      <div className="flex items-center gap-2">
        <Label htmlFor="move-target" className="shrink-0">
          {t('lifecycle.move.target')}
        </Label>
        <NativeSelect
          id="move-target"
          value={target}
          onChange={(e) => {
            setTo(e.target.value)
            setWarning(null)
          }}
        >
          {targets.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </NativeSelect>
      </div>
      {warning ? (
        <p role="alert" className="text-sm text-tone-warning">
          {warning}
        </p>
      ) : null}
    </ConfirmDialog>
  )
}

// Delete removes the domain from the app, which also drops its settings.
export function DeleteDomainDialog({
  app,
  domain,
  open,
  onOpenChange,
}: Props & { app: string; domain: string }) {
  const { t } = useTranslation('domains')
  const edit = useEditAppDomains(app)
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('lifecycle.delete.title', { domain })}
      description={t('lifecycle.delete.body', { app })}
      consequences={[t('lifecycle.delete.dropsSettings')]}
      confirmLabel={t('lifecycle.delete.confirm')}
      pendingLabel={t('lifecycle.pending')}
      tone="destructive"
      requireTyped={domain}
      pending={edit.isPending}
      error={errorText(edit.error)}
      onConfirm={() => {
        edit.mutate(
          { remove: [domain] },
          {
            onSuccess: () => {
              toast.add({
                title: t('lifecycle.delete.done', { domain }),
                type: 'success',
              })
              onOpenChange(false)
            },
          },
        )
      }}
    />
  )
}

export function AttachParkedDialog({
  domain,
  hasSettings = false,
  open,
  onOpenChange,
}: Props & { domain: string; hasSettings?: boolean }) {
  const { t } = useTranslation('domains')
  const { data: apps = [] } = useQuery(appListQueryOptions())
  const [to, setTo] = useState('')
  const [fresh, setFresh] = useState(false)
  const attach = useAttachParkedDomain()
  const target = to || apps[0]?.name || ''
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('lifecycle.attach.title', { domain })}
      description={t('lifecycle.attach.body')}
      confirmLabel={t('lifecycle.attach.confirm')}
      pendingLabel={t('lifecycle.pending')}
      confirmDisabled={target === ''}
      pending={attach.isPending}
      error={errorText(attach.error)}
      onConfirm={() => {
        attach.mutate(
          { domain, app: target, discardSettings: hasSettings && fresh },
          {
            onSuccess: (result) => {
              const kinds = (result.restored_settings ?? []).map((k) =>
                t(`lifecycle.attach.kinds.${k}`, {
                  defaultValue: humanizeSettingKind(k),
                }),
              )
              toast.add({
                title: t('lifecycle.attach.done', { domain, app: target }),
                description:
                  kinds.length > 0
                    ? t('lifecycle.attach.restored', {
                        kinds: kinds.join(', '),
                      })
                    : undefined,
                type: 'success',
              })
              onOpenChange(false)
            },
          },
        )
      }}
    >
      <div className="flex items-center gap-2">
        <Label htmlFor="attach-target" className="shrink-0">
          {t('lifecycle.move.target')}
        </Label>
        <NativeSelect
          id="attach-target"
          value={target}
          onChange={(e) => {
            setTo(e.target.value)
          }}
        >
          {apps.map((a) => (
            <option key={a.name} value={a.name}>
              {a.name}
            </option>
          ))}
        </NativeSelect>
      </div>
      {hasSettings ? (
        <fieldset className="space-y-1">
          <legend className="text-sm font-medium">
            {t('lifecycle.attach.settingsLegend')}
          </legend>
          {[false, true].map((discard) => (
            <label
              key={String(discard)}
              className="flex items-start gap-2 text-sm"
            >
              <input
                type="radio"
                name="attach-settings"
                className="mt-1"
                checked={fresh === discard}
                onChange={() => {
                  setFresh(discard)
                }}
              />
              <span>
                {discard
                  ? t('lifecycle.attach.startFresh')
                  : t('lifecycle.attach.restore')}
              </span>
            </label>
          ))}
        </fieldset>
      ) : null}
    </ConfirmDialog>
  )
}

export function DeleteParkedDialog({
  domain,
  open,
  onOpenChange,
}: Props & { domain: string }) {
  const { t } = useTranslation('domains')
  const del = useDeleteParkedDomain()
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('lifecycle.deleteParked.title', { domain })}
      description={t('lifecycle.deleteParked.body')}
      confirmLabel={t('lifecycle.delete.confirm')}
      pendingLabel={t('lifecycle.pending')}
      tone="destructive"
      requireTyped={domain}
      pending={del.isPending}
      error={errorText(del.error)}
      onConfirm={() => {
        del.mutate(domain, {
          onSuccess: () => {
            toast.add({
              title: t('lifecycle.delete.done', { domain }),
              type: 'success',
            })
            onOpenChange(false)
          },
        })
      }}
    />
  )
}
