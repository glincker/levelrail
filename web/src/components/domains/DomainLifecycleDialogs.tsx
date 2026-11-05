import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import {
  appEnvironmentDomainsQueryOptions,
  useEditAppDomains,
} from '../../queries/appEnvironmentDomains'
import {
  useAttachParkedDomain,
  useDeleteParkedDomain,
  useMoveDomain,
  useParkDomain,
  type TargetKind,
} from '../../queries/parkedDomains'
import {
  attachDropsSettings,
  decodeTarget,
  encodeTarget,
  humanizeSettingKind,
  isNoopMove,
  moveDropsSettings,
  type TargetOption,
} from '../../lib/domainLifecycle'
import { firstTarget, useTargetGroups } from './useTargetGroups'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TargetSelect({
  id,
  value,
  groups,
  label,
  onChange,
}: {
  id: string
  value: string
  label?: string
  groups: { apps: TargetOption[]; sites: TargetOption[] }
  onChange: (value: string) => void
}) {
  const { t } = useTranslation('domains')
  const options = (list: TargetOption[]) =>
    list.map((o) => (
      <option key={encodeTarget(o)} value={encodeTarget(o)}>
        {o.name}
      </option>
    ))
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id} className="shrink-0">
        {label ?? t('lifecycle.move.target')}
      </Label>
      <NativeSelect
        id={id}
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      >
        {groups.apps.length > 0 ? (
          <optgroup label={t('lifecycle.target.apps')}>
            {options(groups.apps)}
          </optgroup>
        ) : null}
        {groups.sites.length > 0 ? (
          <optgroup label={t('lifecycle.target.staticSites')}>
            {options(groups.sites)}
          </optgroup>
        ) : null}
      </NativeSelect>
    </div>
  )
}

// Environment choice for a target app: the default list first, then each
// environment of that app by ID.
export function EnvironmentTargetSelect({
  id,
  app,
  value,
  onChange,
}: {
  id: string
  app: string
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation('domains')
  const { data } = useQuery({
    ...appEnvironmentDomainsQueryOptions(app),
    enabled: app !== '',
  })
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id} className="shrink-0">
        {t('lifecycle.environment.label')}
      </Label>
      <NativeSelect
        id={id}
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      >
        <option value="">{t('lifecycle.environment.default')}</option>
        {(data?.environments ?? []).map((env) => (
          <option key={env.environment_id} value={env.environment_id}>
            {env.name}
          </option>
        ))}
      </NativeSelect>
    </div>
  )
}

function errorText(e: unknown): string | null {
  return e instanceof Error ? e.message : null
}

// Parking snapshots the per-domain settings; attach restores them unless discarded.
export function ParkDomainDialog({
  app,
  domain,
  environment,
  kind = 'app',
  open,
  onOpenChange,
}: Props & {
  app: string
  domain: string
  environment?: string
  kind?: TargetKind
}) {
  const { t } = useTranslation('domains')
  const park = useParkDomain()
  const isStatic = kind === 'static_site'
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('lifecycle.park.title', { domain })}
      description={t('lifecycle.park.body', { app })}
      consequences={[
        isStatic
          ? t('lifecycle.park.staticNoSettings')
          : t('lifecycle.park.savesSettings'),
        t('lifecycle.park.keepsDnsCert'),
      ]}
      confirmLabel={t('lifecycle.park.confirm')}
      pendingLabel={t('lifecycle.pending')}
      focusCancel
      pending={park.isPending}
      error={errorText(park.error)}
      onConfirm={() => {
        park.mutate(
          { app, domain, environment, targetKind: kind },
          {
            onSuccess: (result) => {
              toast.add({
                title: t('lifecycle.park.done', { domain }),
                description: result.message,
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
  fromEnvironment,
  fromKind = 'app',
  open,
  onOpenChange,
}: Props & {
  from: string
  domain: string
  fromEnvironment?: string
  fromKind?: TargetKind
}) {
  const { t } = useTranslation('domains')
  const fromStatic = fromKind === 'static_site'
  const { data: fromSets } = useQuery({
    ...appEnvironmentDomainsQueryOptions(from),
    enabled: !fromStatic,
  })
  const hasEnvironments = (fromSets?.environments.length ?? 0) > 0
  const groups = useTargetGroups(
    { name: from, kind: fromKind },
    hasEnvironments,
  )
  const [to, setTo] = useState('')
  const [toEnvironment, setToEnvironment] = useState('')
  const [warning, setWarning] = useState<string | null>(null)
  const move = useMoveDomain()
  const chosen = decodeTarget(to) ?? firstTarget(groups)
  const target = chosen?.name ?? ''
  const toKind = chosen?.kind ?? 'app'
  const toStatic = toKind === 'static_site'
  const noop =
    chosen !== null &&
    fromKind === toKind &&
    isNoopMove({ from, to: target, fromEnvironment, toEnvironment })
  const drops = moveDropsSettings(fromKind, toKind)
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setWarning(null)
        onOpenChange(next)
      }}
      title={t('lifecycle.move.title', { domain })}
      description={t('lifecycle.move.body', { app: from })}
      consequences={[
        drops
          ? t('lifecycle.move.dropsSettings')
          : t('lifecycle.move.keepsSettings'),
      ]}
      confirmLabel={
        warning ? t('lifecycle.move.confirm') : t('lifecycle.move.check')
      }
      pendingLabel={t('lifecycle.pending')}
      confirmDisabled={target === '' || noop}
      pending={move.isPending}
      error={errorText(move.error)}
      onConfirm={() => {
        move.mutate(
          {
            domain,
            from,
            to: target,
            confirm: warning !== null,
            fromEnvironment,
            toEnvironment,
            fromKind,
            toKind,
          },
          {
            onSuccess: (result) => {
              if (result.applied) {
                toast.add({
                  title: t('lifecycle.move.done', { domain, app: target }),
                  description: result.message,
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
      <TargetSelect
        id="move-target"
        value={chosen ? encodeTarget(chosen) : ''}
        groups={groups}
        onChange={(v) => {
          setTo(v)
          setToEnvironment('')
          setWarning(null)
        }}
      />
      {toStatic ? null : (
        <EnvironmentTargetSelect
          id="move-target-environment"
          app={target}
          value={toEnvironment}
          onChange={(v) => {
            setToEnvironment(v)
            setWarning(null)
          }}
        />
      )}
      {drops ? (
        <p role="alert" className="text-sm text-tone-warning">
          {t('lifecycle.move.dropsWarning')}
        </p>
      ) : null}
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
  const groups = useTargetGroups({ name: '', kind: 'app' }, true)
  const [to, setTo] = useState('')
  const [environment, setEnvironment] = useState('')
  const [fresh, setFresh] = useState(false)
  const attach = useAttachParkedDomain()
  const chosen = decodeTarget(to) ?? firstTarget(groups)
  const target = chosen?.name ?? ''
  const targetKind = chosen?.kind ?? 'app'
  const toStatic = targetKind === 'static_site'
  const drops = attachDropsSettings(hasSettings, targetKind)
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
          {
            domain,
            app: target,
            discardSettings: hasSettings && fresh && !toStatic,
            environment,
            targetKind,
          },
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
                  [
                    kinds.length > 0
                      ? t('lifecycle.attach.restored', {
                          kinds: kinds.join(', '),
                        })
                      : '',
                    result.message ?? '',
                  ]
                    .filter(Boolean)
                    .join(' ') || undefined,
                type: result.message ? 'warning' : 'success',
              })
              onOpenChange(false)
            },
          },
        )
      }}
    >
      <TargetSelect
        id="attach-target"
        value={chosen ? encodeTarget(chosen) : ''}
        groups={groups}
        onChange={(v) => {
          setTo(v)
          setEnvironment('')
        }}
      />
      {toStatic ? null : (
        <EnvironmentTargetSelect
          id="attach-target-environment"
          app={target}
          value={environment}
          onChange={setEnvironment}
        />
      )}
      {drops ? (
        <p role="alert" className="text-sm text-tone-warning">
          {t('lifecycle.attach.dropsWarning')}
        </p>
      ) : null}
      {hasSettings && !toStatic ? (
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
