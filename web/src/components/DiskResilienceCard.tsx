import { useTranslation } from 'react-i18next'
import { BroomGlassIcon, ServerGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import { useDiskResilience, useFreeDiskSpace } from '../queries/diskResilience'
import type { DiskLevel, DiskVolume } from '../queries/diskResilience'

const LEVEL_VARIANT = {
  ok: 'success',
  low: 'warning',
  critical: 'destructive',
  unknown: 'muted',
} as const

const MIB = 1024 * 1024

function formatMib(bytes: number): string {
  return bytes >= 1024 * MIB
    ? `${(bytes / (1024 * MIB)).toFixed(1)} GiB`
    : `${Math.round(bytes / MIB)} MiB`
}

function VolumeRow({ volume }: { volume: DiskVolume }) {
  const { t } = useTranslation('resilience')
  const level: DiskLevel = volume.level
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 py-3">
      <div className="space-y-0.5">
        <p className="text-sm font-medium">
          {t(`disk.volume.${volume.name}`, { defaultValue: volume.name })}
        </p>
        <p className="text-xs text-muted-foreground">{volume.path}</p>
      </div>
      <div className="flex items-center gap-3 text-sm">
        {level !== 'unknown' && (
          <span className="text-muted-foreground">
            {t('disk.free', {
              free: formatMib(volume.free_bytes),
              total: formatMib(volume.total_bytes),
            })}
          </span>
        )}
        <Badge variant={LEVEL_VARIANT[level]}>{t(`disk.level.${level}`)}</Badge>
      </div>
    </li>
  )
}

/** DiskResilienceCard shows free space on the data and Docker volumes, with the Fix when it runs low. */
export function DiskResilienceCard() {
  const { t } = useTranslation('resilience')
  const isRoot = useIsRoot()
  const { data, isLoading, isError } = useDiskResilience()
  const free = useFreeDiskSpace()
  const needsFix = data !== undefined && data.worst !== 'ok'

  function freeSpace() {
    free.mutate(undefined, {
      onSuccess: (r) =>
        toast.add({
          title: t('disk.toast.freed', { size: formatMib(r.reclaimed_bytes) }),
          type: 'success',
        }),
      onError: (error) =>
        toast.add({
          title: t('disk.toast.errorTitle'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card id="disk-resilience">
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <CardTitle className="flex items-center gap-2">
              <ServerGlassIcon size="1.2em" aria-hidden="true" />
              {t('disk.title')}
            </CardTitle>
            <CardDescription>{t('disk.description')}</CardDescription>
          </div>
          {isRoot && data?.free_space_available && (
            <Button
              variant={needsFix ? 'default' : 'outline'}
              size="sm"
              onClick={freeSpace}
              disabled={free.isPending}
            >
              <BroomGlassIcon size="1.2em" aria-hidden="true" />
              {free.isPending ? t('disk.freeing') : t('disk.freeAction')}
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading && <ListSkeleton rows={2} />}
        {isError && (
          <Alert variant="destructive">
            <AlertDescription>{t('disk.loadError')}</AlertDescription>
          </Alert>
        )}
        {data?.refusing && (
          <Alert variant="destructive">
            <AlertDescription>{t('disk.refusing')}</AlertDescription>
          </Alert>
        )}
        {data && (
          <ul className="divide-y divide-border">
            {data.volumes.map((v) => (
              <VolumeRow key={v.name} volume={v} />
            ))}
          </ul>
        )}
        {data && (
          <p className="text-xs text-muted-foreground">
            {t('disk.thresholds', {
              low: formatMib(data.thresholds.low_free_bytes),
              critical: formatMib(data.thresholds.critical_free_bytes),
            })}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
