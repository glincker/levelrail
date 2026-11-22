import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { AwsWizardState, ScopeIssue } from '../../lib/cloudAccounts'

// Example values, not translatable text.
const EX_ADDR = 'https://vault.example.com:8200'
const EX_REF = 'secret://vault/levelrail#token'

export function AwsLinkVaultFields({
  state,
  onChange,
  issues,
}: Readonly<{
  state: AwsWizardState
  onChange: (patch: Partial<AwsWizardState>) => void
  issues: ReadonlySet<ScopeIssue>
}>) {
  const { t } = useTranslation('cloudAccounts')
  const uid = useId()
  const field = (
    key:
      | 'accountId'
      | 'vaultAddress'
      | 'vaultMount'
      | 'vaultRole'
      | 'vaultNamespace'
      | 'vaultTokenRef',
    label: string,
    issue?: ScopeIssue,
    placeholder?: string,
  ) => (
    <div className="space-y-1">
      <Label htmlFor={`${uid}-${key}`}>{label}</Label>
      <Input
        id={`${uid}-${key}`}
        value={state[key]}
        placeholder={placeholder}
        className="font-mono"
        aria-invalid={issue ? issues.has(issue) : undefined}
        onChange={(e) => {
          onChange({ [key]: e.target.value.trim() })
        }}
      />
      {issue && issues.has(issue) ? (
        <p role="alert" className="text-xs text-destructive">
          {t(`wizard.issues.${issue}`)}
        </p>
      ) : null}
    </div>
  )
  return (
    <fieldset className="space-y-3 rounded-lg border border-border p-3">
      <label className="flex cursor-pointer items-start gap-2">
        <Checkbox
          className="mt-0.5"
          checked={state.useVault}
          onCheckedChange={(v) => {
            onChange({ useVault: v === true })
          }}
        />
        <span className="min-w-0 text-sm">
          <span className="block font-medium">{t('wizard.vault.use')}</span>
          <span className="block text-xs text-muted-foreground">
            {t('wizard.vault.hint')}
          </span>
        </span>
      </label>
      {state.useVault ? (
        <div className="grid gap-3 sm:grid-cols-2">
          {field('accountId', t('wizard.vault.accountId'), 'badAccountId')}
          {field(
            'vaultAddress',
            t('wizard.vault.address'),
            'badVaultAddress',
            EX_ADDR,
          )}
          {field('vaultMount', t('wizard.vault.mount'))}
          {field('vaultRole', t('wizard.vault.role'), 'badVaultRole')}
          {field('vaultNamespace', t('wizard.vault.namespace'))}
          {field(
            'vaultTokenRef',
            t('wizard.vault.tokenRef'),
            'badVaultToken',
            EX_REF,
          )}
        </div>
      ) : null}
    </fieldset>
  )
}
