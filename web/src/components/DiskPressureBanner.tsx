import { useState } from 'react'
import { WarningCircleIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { assessDiskPressure } from '../lib/diskPressure'
import { formatBytes } from '../lib/format'
import { useDockerHealthPoll } from '../queries/systemStatus'
import { CleanUpDockerDialog } from './CleanUpDockerDialog'

export function DiskPressureBanner() {
  const { data } = useDockerHealthPoll()
  const [dismissedLevel, setDismissedLevel] = useState<string>()
  const pressure = data ? assessDiskPressure(data) : undefined

  if (!pressure || pressure.level === 'ok') {
    return null
  }
  if (dismissedLevel === pressure.level) {
    return null
  }

  return (
    <Alert
      variant={pressure.level === 'critical' ? 'destructive' : 'default'}
      className="rounded-none border-x-0 border-t-0"
    >
      <WarningCircleIcon />
      <AlertTitle>
        {pressure.level === 'critical'
          ? 'Disk almost full'
          : 'Disk space is getting low'}
      </AlertTitle>
      <AlertDescription>
        {formatBytes(pressure.freeBytes)} free (
        {pressure.freePercent.toFixed(1)}
        %) on the data volume.{' '}
        {pressure.reclaimableBytes > 0
          ? `About ${formatBytes(pressure.reclaimableBytes)} of unused Docker data can be reclaimed. `
          : ''}
        Builds and deploys may fail if it runs out.
        <span className="mt-2 block">
          <CleanUpDockerDialog />
        </span>
      </AlertDescription>
      <AlertAction>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Dismiss for now"
          title="Dismiss for now"
          onClick={() => {
            setDismissedLevel(pressure.level)
          }}
        >
          <XIcon />
        </Button>
      </AlertAction>
    </Alert>
  )
}
