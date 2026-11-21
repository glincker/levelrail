import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  AWS_CAPABILITIES,
  scopeIssues,
  toggleCapability,
  type AwsWizardState,
  type ScopeIssue,
} from '../../lib/cloudAccounts'
import type { AwsControlIdentity } from '../../types/cloudAccounts'

// Example ARNs and ids, not translatable text.
const EX_0 = 'eu-west-1, us-east-1'
const EX_1 = 'arn:aws:iam::123456789012:role/levelrail'
const EX_2 = 'arn:aws:iam::123456789012:policy/guard'

export function AwsLinkScopeStep({
  state,
  onChange,
  showIssues,
  identity,
}: Readonly<{
  state: AwsWizardState
  onChange: (patch: Partial<AwsWizardState>) => void
  showIssues: boolean
  identity?: AwsControlIdentity
}>) {
  const { t } = useTranslation('cloudAccounts')
  const uid = useId()
  const issues = new Set<ScopeIssue>(showIssues ? scopeIssues(state) : [])
  const err = (i: ScopeIssue) =>
    issues.has(i) ? (
      <p id={`${uid}-${i}`} role="alert" className="text-xs text-destructive">
        {t(`wizard.issues.${i}`)}
      </p>
    ) : null

  return (
    <div className="space-y-4">
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">
          {t('wizard.scope.tiers')}
        </legend>
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {AWS_CAPABILITIES.map((cap) => {
            const risky = cap === 'destructive'
            return (
              <li key={cap}>
                <label
                  className={
                    risky
                      ? 'flex h-full cursor-pointer items-start gap-2 rounded-lg border border-destructive/40 p-2.5'
                      : 'flex h-full cursor-pointer items-start gap-2 rounded-lg border border-border p-2.5'
                  }
                >
                  <Checkbox
                    className="mt-0.5"
                    checked={state.capabilities.includes(cap)}
                    onCheckedChange={() => {
                      onChange({
                        capabilities: toggleCapability(state.capabilities, cap),
                      })
                    }}
                  />
                  <span className="min-w-0 text-sm">
                    <span className="block font-medium">
                      {t(`capabilities.${cap}.name`)}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {t(`capabilities.${cap}.hint`)}
                    </span>
                  </span>
                </label>
              </li>
            )
          })}
        </ul>
        {err('noCapabilities')}
      </fieldset>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1">
          <Label htmlFor={`${uid}-regions`}>{t('wizard.scope.regions')}</Label>
          <Input
            id={`${uid}-regions`}
            value={state.regionsText}
            placeholder={EX_0}
            aria-invalid={issues.has('badRegions')}
            onChange={(e) => {
              onChange({ regionsText: e.target.value })
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('wizard.scope.regionsHint')}
          </p>
          {err('badRegions')}
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${uid}-zones`}>{t('wizard.scope.zones')}</Label>
          <Input
            id={`${uid}-zones`}
            value={state.zonesText}
            aria-invalid={issues.has('needsZones') || issues.has('badZones')}
            onChange={(e) => {
              onChange({ zonesText: e.target.value })
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('wizard.scope.zonesHint')}
          </p>
          {err('needsZones')}
          {err('badZones')}
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${uid}-region`}>{t('wizard.scope.region')}</Label>
          <Input
            id={`${uid}-region`}
            value={state.region}
            aria-invalid={issues.has('badRegion')}
            onChange={(e) => {
              onChange({ region: e.target.value.trim() })
            }}
          />
          {err('badRegion')}
        </div>
        <div className="space-y-1 sm:col-span-2">
          <Label htmlFor={`${uid}-principal`}>
            {t('wizard.scope.principal')}
          </Label>
          <Input
            id={`${uid}-principal`}
            value={state.trustedPrincipalArn}
            placeholder={EX_1}
            className="font-mono"
            aria-invalid={issues.has('badPrincipal')}
            onChange={(e) => {
              onChange({ trustedPrincipalArn: e.target.value })
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('wizard.scope.principalHint')}
          </p>
          {identity?.configured ? (
            <p className="text-xs text-muted-foreground">
              {t('wizard.scope.principalPrefilled', {
                account: identity.account,
              })}
            </p>
          ) : null}
          {identity && !identity.configured ? (
            <p className="text-xs text-muted-foreground">
              {t('wizard.scope.principalNotConfigured')}
            </p>
          ) : null}
          {err('badPrincipal')}
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${uid}-guard`}>{t('wizard.scope.guard')}</Label>
          <Input
            id={`${uid}-guard`}
            value={state.guardPolicyArn}
            placeholder={EX_2}
            className="font-mono"
            aria-invalid={issues.has('badGuard')}
            onChange={(e) => {
              onChange({ guardPolicyArn: e.target.value })
            }}
          />
          {err('badGuard')}
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${uid}-role`}>{t('wizard.scope.roleName')}</Label>
          <Input
            id={`${uid}-role`}
            value={state.roleName}
            placeholder={t('wizard.scope.roleNamePlaceholder')}
            onChange={(e) => {
              onChange({ roleName: e.target.value.trim() })
            }}
          />
        </div>
      </div>
    </div>
  )
}
