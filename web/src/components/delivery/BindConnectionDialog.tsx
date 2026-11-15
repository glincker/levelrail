import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import {
  connectionsForRole,
  isBindingRole,
  rolesForConnection,
  type BindingRole,
} from '../../lib/connectionBindings'
import { useAppListOptional } from '../../queries/apps'
import { useBindConnection } from '../../queries/bindings'
import type { Connection } from '../../types/connections'

function rulesError(text: string): boolean {
  if (text.trim() === '') return false
  try {
    const v: unknown = JSON.parse(text)
    return typeof v !== 'object' || v === null || Array.isArray(v)
  } catch {
    return true
  }
}

interface BindProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Connection to bind; without it the dialog offers the ones that fit `role`. */
  connection?: Connection
  connections?: readonly Connection[]
  /** Fixed target app; without it the dialog offers an app picker. */
  app?: string
  /** Preselected role; must be one the connection can fill. */
  role?: string
}

// POST /api/v1/apps/{name}/bindings. The server's refusal text is shown as sent.
export function BindConnectionDialog({
  open,
  onOpenChange,
  connection,
  connections = [],
  app,
  role,
}: Readonly<BindProps>) {
  const { t } = useTranslation('delivery')
  const ids = { app: useId(), conn: useId(), role: useId(), rules: useId() }
  const presetRole = role && isBindingRole(role) ? role : undefined
  const choices = presetRole
    ? connectionsForRole(connections, presetRole)
    : connections.filter((c) => rolesForConnection(c).length > 0)
  const [pickedRef, setPickedRef] = useState('')
  const chosen = connection ?? choices.find((c) => c.ref === pickedRef)
  const roles = chosen ? rolesForConnection(chosen) : []
  const apps = useAppListOptional()
  const bind = useBindConnection()
  const [pickedApp, setPickedApp] = useState('')
  const [pickedRole, setPickedRole] = useState<string>(presetRole ?? '')
  const effectiveRole = roles.some((r) => r === pickedRole)
    ? pickedRole
    : (roles[0] ?? '')
  const [rules, setRules] = useState('')
  const target = app ?? pickedApp
  const badRules = rulesError(rules)

  function handleOpenChange(next: boolean) {
    onOpenChange(next)
    if (!next) {
      bind.reset()
      setRules('')
    }
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={handleOpenChange}
      title={
        chosen
          ? t('bind.title', { name: chosen.label })
          : t('bind.titleGeneric')
      }
      description={t('bind.description')}
      confirmLabel={t('bind.confirm')}
      pendingLabel={t('bind.binding')}
      pending={bind.isPending}
      confirmDisabled={
        target === '' || !chosen || effectiveRole === '' || badRules
      }
      error={bind.isError ? bind.error.message : null}
      onConfirm={() => {
        bind.mutate(
          { app: target, ref: chosen?.ref ?? '', role: effectiveRole, rules },
          {
            onSuccess: () => {
              onOpenChange(false)
              toast.add({
                title: t('bind.toast', {
                  name: chosen?.label ?? '',
                  app: target,
                }),
                type: 'success',
              })
            },
          },
        )
      }}
    >
      <div className="space-y-3">
        {app === undefined ? (
          <div className="space-y-1.5">
            <Label htmlFor={ids.app}>{t('bind.app')}</Label>
            <NativeSelect
              id={ids.app}
              className="w-full"
              value={pickedApp}
              onChange={(e) => setPickedApp(e.target.value)}
            >
              <option value="">{t('bind.appPlaceholder')}</option>
              {(apps.data ?? []).map((a) => (
                <option key={a.name} value={a.name}>
                  {a.name}
                </option>
              ))}
            </NativeSelect>
          </div>
        ) : null}
        {connection === undefined ? (
          <div className="space-y-1.5">
            <Label htmlFor={ids.conn}>{t('bind.connection')}</Label>
            <NativeSelect
              id={ids.conn}
              className="w-full"
              value={pickedRef}
              onChange={(e) => setPickedRef(e.target.value)}
            >
              <option value="">
                {choices.length === 0
                  ? t('bind.noConnections')
                  : t('bind.connectionPlaceholder')}
              </option>
              {choices.map((c) => (
                <option key={c.ref} value={c.ref}>
                  {c.label}
                </option>
              ))}
            </NativeSelect>
          </div>
        ) : null}
        <div className="space-y-1.5">
          <Label htmlFor={ids.role}>{t('bind.role')}</Label>
          <NativeSelect
            id={ids.role}
            className="w-full"
            value={effectiveRole}
            disabled={roles.length === 0}
            onChange={(e) => setPickedRole(e.target.value)}
          >
            {roles.map((r: BindingRole) => (
              <option key={r} value={r}>
                {t(`bind.roles.${r}`)}
              </option>
            ))}
          </NativeSelect>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={ids.rules}>{t('bind.rules')}</Label>
          <Input
            id={ids.rules}
            value={rules}
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
            placeholder='{"branch":"main"}'
            aria-invalid={badRules || undefined}
            onChange={(e) => setRules(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            {badRules ? t('bind.rulesInvalid') : t('bind.rulesHint')}
          </p>
        </div>
      </div>
    </ConfirmDialog>
  )
}
