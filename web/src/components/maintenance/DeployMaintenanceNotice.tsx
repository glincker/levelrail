import { useTranslation } from 'react-i18next'
import { InfoGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useAppInMaintenance } from '../../hooks/useAppInMaintenance'

// Tells the operator a deploy neither ends nor is blocked by maintenance.
export function DeployMaintenanceNotice({ appName }: { appName: string }) {
  const { t } = useTranslation('maintenance')
  const inMaintenance = useAppInMaintenance(appName)
  if (!inMaintenance) return null
  return (
    <Alert>
      <InfoGlassIcon className="text-tone-info" size="1.4em" />
      <AlertDescription>{t('deployNotice.text')}</AlertDescription>
    </Alert>
  )
}
