import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  PauseCircleIcon,
  ShieldCheckIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import {
  useUpdateAiControl,
  type AiControlMode,
  type AiControlSettings,
} from '../queries/aiControl'
import { RevokeAgentTokensDialog } from './RevokeAgentTokensDialog'
import { SettingsFormAlerts } from './SettingsCard'

export function AiControlCard({ settings }: { settings: AiControlSettings }) {
  const { t } = useTranslation('settings')
  const update = useUpdateAiControl()
  const [mode, setMode] = useState<AiControlMode>(settings.mode)
  const [kinds, setKinds] = useState<string[]>(settings.allowed_env_kinds)

  const modes: { value: AiControlMode; label: string; description: string }[] =
    [
      {
        value: 'off',
        label: t('aiControl.modes.off.label'),
        description: t('aiControl.modes.off.description'),
      },
      {
        value: 'observe',
        label: t('aiControl.modes.observe.label'),
        description: t('aiControl.modes.observe.description'),
      },
      {
        value: 'operate',
        label: t('aiControl.modes.operate.label'),
        description: t('aiControl.modes.operate.description'),
      },
      {
        value: 'admin',
        label: t('aiControl.modes.admin.label'),
        description: t('aiControl.modes.admin.description'),
      },
    ]
  const kindLabels: Record<string, string> = {
    dev: t('aiControl.kinds.dev'),
    test: t('aiControl.kinds.test'),
    uat: t('aiControl.kinds.uat'),
    production: t('aiControl.kinds.production'),
    preview: t('aiControl.kinds.preview'),
    custom: t('aiControl.kinds.custom'),
  }

  const dirty =
    mode !== settings.mode ||
    [...kinds].sort().join(',') !==
      [...settings.allowed_env_kinds].sort().join(',')
  const kindsApply = mode === 'operate' || mode === 'admin'

  function toggleKind(kind: string, checked: boolean) {
    setKinds((cur) =>
      checked ? [...cur, kind] : cur.filter((k) => k !== kind),
    )
  }

  function save() {
    update.mutate(
      { mode, allowed_env_kinds: kinds },
      {
        onSuccess: () =>
          toast.add({ title: t('aiControl.saved'), type: 'success' }),
      },
    )
  }

  function pause() {
    update.mutate(
      { mode: 'off', allowed_env_kinds: settings.allowed_env_kinds },
      {
        onSuccess: (next) => {
          setMode(next.mode)
          toast.add({ title: t('aiControl.paused'), type: 'success' })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckIcon className="size-4" aria-hidden="true" />
          {t('aiControl.title')}
        </CardTitle>
        <CardDescription>{t('aiControl.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div
          role="radiogroup"
          aria-label={t('aiControl.modeLabel')}
          className="grid gap-2"
        >
          {modes.map((m) => {
            const unavailable = m.value === 'admin' && !settings.admin_available
            const selected = mode === m.value
            return (
              <label
                key={m.value}
                className={cn(
                  'flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm',
                  selected ? 'border-primary bg-muted' : 'border-border',
                  unavailable && 'cursor-not-allowed opacity-60',
                )}
              >
                <input
                  type="radio"
                  name="ai-control-mode"
                  value={m.value}
                  checked={selected}
                  disabled={unavailable}
                  onChange={() => setMode(m.value)}
                  className="mt-1"
                />
                <span className="space-y-0.5">
                  <span className="block font-medium">{m.label}</span>
                  <span className="block text-muted-foreground">
                    {m.description}
                  </span>
                  {unavailable ? (
                    <span className="block text-muted-foreground">
                      {t('aiControl.adminUnavailable')}
                    </span>
                  ) : null}
                </span>
              </label>
            )
          })}
        </div>

        <fieldset className="space-y-2" disabled={!kindsApply}>
          <legend className="text-sm font-medium">
            {t('aiControl.envKindsLabel')}
          </legend>
          <p className="text-sm text-muted-foreground">
            {t('aiControl.envKindsDescription')}
          </p>
          <div className="flex flex-wrap gap-4">
            {settings.env_kinds.map((kind) => (
              <label key={kind} className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={kinds.includes(kind)}
                  onCheckedChange={(checked) => toggleKind(kind, checked)}
                  disabled={!kindsApply}
                />
                {kindLabels[kind] ?? kind}
              </label>
            ))}
          </div>
        </fieldset>

        <SettingsFormAlerts
          alerts={[
            update.isError && { key: 'update', message: update.error.message },
          ]}
        />

        <div className="flex flex-wrap items-center gap-2">
          <Button onClick={save} disabled={!dirty || update.isPending}>
            {update.isPending ? t('aiControl.saving') : t('aiControl.save')}
          </Button>
          <Button
            variant="destructive"
            onClick={pause}
            disabled={update.isPending || settings.mode === 'off'}
          >
            <PauseCircleIcon className="size-4" aria-hidden="true" />
            {t('aiControl.pause')}
          </Button>
        </div>

        <div className="space-y-2 border-t pt-4 text-sm">
          <p className="font-medium">{t('aiControl.agentTokens')}</p>
          <p className="text-muted-foreground">
            {t('aiControl.agentTokensCount', {
              count: settings.agent_token_count,
            })}
          </p>
          {settings.updated_by ? (
            <p className="text-muted-foreground">
              {t('aiControl.updatedBy', { name: settings.updated_by })}
            </p>
          ) : null}
          <div className="flex flex-wrap items-center gap-3">
            <RevokeAgentTokensDialog
              disabled={settings.agent_token_count === 0}
            />
            <Link
              to="/settings/audit-log"
              className="text-sm underline underline-offset-4"
            >
              {t('aiControl.viewAudit')}
            </Link>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
