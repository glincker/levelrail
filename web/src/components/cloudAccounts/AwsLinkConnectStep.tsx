import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isRoleArn, type AwsWizardState } from '../../lib/cloudAccounts'
import type {
  CloudCapabilityResult,
  CloudVerifyResult,
} from '../../types/cloudAccounts'
import { CapabilityResults } from './CapabilityResults'

// Example ARNs and ids, not translatable text.
const EX_0 = 'arn:aws:iam::123456789012:role/LevelrailAccess'

export function AwsLinkConnectStep({
  state,
  onChange,
  locked,
  verify,
  probe,
  error,
}: Readonly<{
  state: AwsWizardState
  onChange: (patch: Partial<AwsWizardState>) => void
  /** Once the account exists the inputs are read only. */
  locked: boolean
  verify: CloudVerifyResult | null
  probe: readonly CloudCapabilityResult[] | null
  error: string | null
}>) {
  const { t } = useTranslation('cloudAccounts')
  const uid = useId()
  const badArn = state.roleArn.trim() !== '' && !isRoleArn(state.roleArn)
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-[1fr_16rem]">
        <div className="space-y-1">
          <Label htmlFor={`${uid}-arn`}>{t('wizard.connect.roleArn')}</Label>
          <Input
            id={`${uid}-arn`}
            value={state.roleArn}
            disabled={locked}
            className="font-mono"
            placeholder={EX_0}
            aria-invalid={badArn}
            onChange={(e) => {
              onChange({ roleArn: e.target.value.trim() })
            }}
          />
          {badArn ? (
            <p role="alert" className="text-xs text-destructive">
              {t('wizard.connect.badArn')}
            </p>
          ) : null}
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${uid}-label`}>{t('wizard.connect.label')}</Label>
          <Input
            id={`${uid}-label`}
            value={state.label}
            disabled={locked}
            onChange={(e) => {
              onChange({ label: e.target.value })
            }}
          />
        </div>
      </div>
      {error ? (
        <Alert variant="danger">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {verify ? (
        <Alert variant={verify.ok ? 'success' : 'danger'}>
          <AlertDescription>
            <span role="status">
              {verify.message}
              {verify.identity
                ? ` ${t('wizard.connect.identity', {
                    account: verify.identity.account,
                  })}`
                : ''}
              {verify.needs ? ` ${verify.needs}` : ''}
            </span>
          </AlertDescription>
        </Alert>
      ) : null}
      {probe ? <CapabilityResults results={probe} /> : null}
    </div>
  )
}
