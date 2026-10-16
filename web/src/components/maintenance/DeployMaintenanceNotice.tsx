import { useTranslation } from 'react-i18next'
import { InfoIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useAppInMaintenance } from '../../hooks/useAppInMaintenance'

// Tells the operator a deploy neither ends nor is blocked by maintenance.
export function DeployMaintenanceNotice({ appName }: { appName: string }) {
  const { t } = useTranslation('maintenance')
  const inMaintenance = useAppInMaintenance(appName)
  if (!inMaintenance) return null
  return (
    <Alert>
      <InfoIcon />
      <AlertDescription>{t('deployNotice.text')}</AlertDescription>
    </Alert>
  )
}
