import { useTranslation } from 'react-i18next'
import { ShieldAlertGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { InfoTip, SkeletonLine } from './kit'
import {
  useSecretScanSettings,
  useSetSecretScanPolicy,
  useUnallowSecretFingerprint,
} from '../queries/secretScan'
import type { SecretScanPolicy, SecretScanSettings } from '../types/secretScan'

const POLICIES: SecretScanPolicy[] = ['off', 'warn', 'block']

function PolicySelector({
  value,
  disabled,
  onChange,
}: {
  value: SecretScanPolicy
  disabled: boolean
  onChange: (policy: SecretScanPolicy) => void
}) {
  const { t } = useTranslation('secretScan')
  return (
    <div
      role="radiogroup"
      aria-label={t('settings.policyLabel')}
      className="grid gap-2"
    >
      {POLICIES.map((p) => {
        const id = `secret-scan-policy-${p}`
        const selected = value === p
        return (
          <label
            key={p}
            htmlFor={id}
            className={cn(
              'grid cursor-pointer grid-cols-[auto_1fr] items-start gap-x-3 rounded-md border p-3 text-sm font-medium text-foreground',
              selected ? 'border-primary bg-muted/50' : 'border-border',
              disabled && 'cursor-not-allowed opacity-60',
            )}
          >
            <input
              id={id}
              type="radio"
              name="secret-scan-policy"
              className="mt-1 row-span-2 accent-primary"
              checked={selected}
              disabled={disabled}
              onChange={() => onChange(p)}
            />
            {t(`settings.policy.${p}.label`)}
            <span className="font-normal text-muted-foreground">
              {t(`settings.policy.${p}.summary`)}
            </span>
          </label>
        )
      })}
    </div>
  )
}

function AllowList({
  appName,
  settings,
}: {
  appName: string
  settings: SecretScanSettings
}) {
  const { t } = useTranslation('secretScan')
  const remove = useUnallowSecretFingerprint(appName)
  function onRemove(fingerprint: string) {
    remove.mutate(fingerprint, {
      onSuccess: () =>
        toast.add({ title: t('settings.toastRemoved'), type: 'success' }),
      onError: (error) =>
        toast.add({
          title: t('settings.toastRemoveFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t('settings.allowListTitle')}</p>
      {settings.allow_list.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('settings.allowListEmpty')}
        </p>
      ) : (
        <ul className="space-y-2">
          {settings.allow_list.map((e) => (
            <li
              key={e.fingerprint}
              className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border p-2 text-sm"
            >
              <div className="min-w-0">
                <p className="font-mono text-xs">{e.fingerprint}</p>
                <p className="text-muted-foreground">{e.reason}</p>
                <p className="text-xs text-muted-foreground">
                  {t('settings.allowedBy', {
                    actor: e.actor,
                    date: new Date(e.created_at).toLocaleDateString(),
                  })}
                </p>
              </div>
              <Button
                size="sm"
                variant="outline"
                onClick={() => onRemove(e.fingerprint)}
                disabled={remove.isPending}
              >
                {t('settings.remove')}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function SettingsBody({
  appName,
  settings,
}: {
  appName: string
  settings: SecretScanSettings
}) {
  const { t } = useTranslation('secretScan')
  const save = useSetSecretScanPolicy(appName)
  function change(policy: SecretScanPolicy) {
    save.mutate(policy, {
      onSuccess: () =>
        toast.add({ title: t('settings.toastSaved'), type: 'success' }),
      onError: (error) =>
        toast.add({
          title: t('settings.toastFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }
  return (
    <div className="space-y-5">
      <PolicySelector
        value={settings.policy}
        disabled={save.isPending}
        onChange={change}
      />
      <AllowList appName={appName} settings={settings} />
    </div>
  )
}

// SecretScanSettingsCard sets the per-app secret scan policy and manages its
// allow-list (GET/PUT /api/v1/apps/{name}/secret-scan).
export function SecretScanSettingsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('secretScan')
  const settings = useSecretScanSettings(appName)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldAlertGlassIcon className="size-5 text-muted-foreground" />
          {t('settings.title')}
          <InfoTip label={t('settings.title')}>{t('settings.about')}</InfoTip>
        </CardTitle>
        <CardDescription>{t('settings.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {settings.isPending ? (
          <div className="space-y-2" aria-label={t('settings.loading')}>
            <SkeletonLine width="60%" />
            <SkeletonLine width="40%" />
          </div>
        ) : settings.isError ? (
          <p className="text-sm text-muted-foreground">
            {t('settings.unavailable')}
          </p>
        ) : (
          <SettingsBody appName={appName} settings={settings.data} />
        )}
      </CardContent>
    </Card>
  )
}
