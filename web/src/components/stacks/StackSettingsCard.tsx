import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useUpdateStackSettings } from '../../queries/stacks'
import type { Stack, StackSettingsInput } from '../../types/stacks'

function ToggleRow({
  label,
  hint,
  checked,
  disabled,
  onChange,
}: Readonly<{
  label: string
  hint: string
  checked: boolean
  disabled: boolean
  onChange: (next: boolean) => void
}>) {
  return (
    <div className="flex items-center justify-between gap-3">
      <div className="min-w-0">
        <div className="text-sm font-medium">{label}</div>
        <div className="text-xs text-muted-foreground">{hint}</div>
      </div>
      <Switch
        checked={checked}
        disabled={disabled}
        aria-label={label}
        onCheckedChange={onChange}
      />
    </div>
  )
}

// Root-only on the server; read-only here for everyone else.
export function StackSettingsCard({
  stack,
  isRoot,
}: Readonly<{ stack: Stack; isRoot: boolean }>) {
  const { t } = useTranslation('stacks')
  const update = useUpdateStackSettings(stack.name)
  const disabled = !isRoot || update.isPending

  function save(input: StackSettingsInput) {
    update.mutate(input, {
      onSuccess: () => {
        toast.add({ title: t('settings.saved'), type: 'success' })
      },
      onError: (e) => {
        toast.add({
          title: t('settings.failed'),
          description: e.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <section
      className="space-y-3 rounded-xl border border-border bg-card p-3 shadow-sm"
      aria-label={t('settings.title')}
    >
      <h3 className="text-sm font-semibold">{t('settings.title')}</h3>
      <ToggleRow
        label={t('settings.allowPrune')}
        hint={t('settings.allowPruneHint')}
        checked={stack.allow_prune}
        disabled={disabled}
        onChange={(next) => {
          save({ allow_prune: next })
        }}
      />
      <ToggleRow
        label={t('settings.requireApproval')}
        hint={t('settings.requireApprovalHint')}
        checked={stack.require_approval}
        disabled={disabled}
        onChange={(next) => {
          save({ require_approval: next })
        }}
      />
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="text-sm font-medium">{t('settings.driftPolicy')}</div>
          <div className="text-xs text-muted-foreground">
            {t('settings.driftHint')}
          </div>
        </div>
        <div
          className="flex gap-1"
          role="group"
          aria-label={t('settings.driftPolicy')}
        >
          {(['warn', 'lock'] as const).map((p) => (
            <Button
              key={p}
              type="button"
              size="xs"
              variant={stack.drift_policy === p ? 'default' : 'outline'}
              aria-pressed={stack.drift_policy === p}
              disabled={disabled}
              onClick={() => {
                if (stack.drift_policy !== p) save({ drift_policy: p })
              }}
            >
              {t(p === 'warn' ? 'settings.driftWarn' : 'settings.driftLock')}
            </Button>
          ))}
        </div>
      </div>
      {isRoot ? null : (
        <p className="text-xs text-muted-foreground">
          {t('settings.rootOnly')}
        </p>
      )}
    </section>
  )
}
