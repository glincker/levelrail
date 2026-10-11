import { useTranslation } from 'react-i18next'
import { InfoIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import type { EnvSettings } from '../../queries/envProtection'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

// Explains why values cannot be revealed (or can be without confirming).
export function EnvPolicyNotice({ settings }: { settings?: EnvSettings }) {
  const { t } = useTranslation('envEditor')
  if (!settings) return null
  if (!settings.policy_valid) {
    return (
      <Alert>
        <WarningIcon aria-hidden />
        <AlertTitle>{t('policy.invalidTitle')}</AlertTitle>
        <AlertDescription>{t('policy.invalidBody')}</AlertDescription>
      </Alert>
    )
  }
  if (settings.reveal_policy === 'disabled') {
    return (
      <Alert>
        <InfoIcon aria-hidden />
        <AlertTitle>{t('policy.disabledTitle')}</AlertTitle>
        <AlertDescription>{t('policy.disabledBody')}</AlertDescription>
      </Alert>
    )
  }
  if (settings.reveal_policy === 'allow') {
    return (
      <Alert>
        <WarningIcon aria-hidden />
        <AlertTitle>{t('policy.allowTitle')}</AlertTitle>
        <AlertDescription>{t('policy.allowBody')}</AlertDescription>
      </Alert>
    )
  }
  if (settings.secrets_write_only) {
    return (
      <Alert>
        <InfoIcon aria-hidden />
        <AlertTitle>{t('policy.writeOnlyTitle')}</AlertTitle>
        <AlertDescription>{t('policy.writeOnlyBody')}</AlertDescription>
      </Alert>
    )
  }
  return null
}
