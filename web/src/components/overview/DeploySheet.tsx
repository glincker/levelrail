import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DeployMaintenanceNotice } from '../maintenance/DeployMaintenanceNotice'
import { SmartDeployBody, type DeployMode } from '../deploy/SmartDeployBody'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

export type DeployTab = 'existing-image' | 'build-from-source'

export function DeploySheet({
  appName,
  tab,
  onClose,
}: {
  appName: string
  tab: DeployTab | null
  onClose: () => void
}) {
  const { t } = useTranslation('smartDeploy')
  const [mode, setMode] = useState<DeployMode>('auto')
  return (
    <Sheet
      open={tab !== null}
      onOpenChange={(open) => {
        if (!open) {
          setMode('auto')
          onClose()
        }
      }}
    >
      <SheetContent
        side="right"
        className="overflow-y-auto data-[side=right]:w-full data-[side=right]:sm:max-w-lg"
      >
        <SheetHeader>
          <SheetTitle>{t('title')}</SheetTitle>
          <SheetDescription>{t('description')}</SheetDescription>
        </SheetHeader>
        <div className="space-y-3 p-4 pt-0">
          <DeployMaintenanceNotice appName={appName} />
          {tab ? (
            <SmartDeployBody
              appName={appName}
              tab={tab}
              mode={mode}
              onModeChange={setMode}
              onClose={onClose}
            />
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
