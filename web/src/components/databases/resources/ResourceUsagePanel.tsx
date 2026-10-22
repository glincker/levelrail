import { useTranslation } from 'react-i18next'
import { GaugeIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatBytes, formatNanoCpus } from '../../../lib/format'
import type { DatabaseResource } from '../../../types/databaseDetail'
import { headroom } from './sizingHelpers'
import { useResourceUsage } from './useResourceUsage'

export function ResourceUsagePanel({
  database,
}: {
  database: DatabaseResource
}) {
  const { t } = useTranslation('databases')
  const {
    running: enabled,
    memNow,
    memPeak,
    cpuNow,
    cpuPeak,
  } = useResourceUsage(database)
  const memLimit = database.resources?.memory_bytes ?? 0
  const cpuLimit = database.resources?.nano_cpus ?? 0
  const memHeadroom = headroom(memPeak, memLimit)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GaugeIcon className="size-4" aria-hidden="true" />
          {t('sizing.usageTitle')}
        </CardTitle>
        <CardDescription>{t('sizing.usageDescription')}</CardDescription>
      </CardHeader>
      <CardContent>
        {!enabled ? (
          <p className="text-sm text-muted-foreground">{t('sizing.stopped')}</p>
        ) : memNow === null && cpuNow === null ? (
          <p className="text-sm text-muted-foreground">{t('sizing.noData')}</p>
        ) : (
          <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('sizing.memory')}
              </dt>
              <dd className="mt-1 text-sm text-foreground">
                {t('sizing.usedOfLimit', {
                  used: formatBytes(memNow),
                  limit:
                    memLimit > 0 ? formatBytes(memLimit) : t('sizing.noLimit'),
                })}
              </dd>
              <dd className="text-xs text-muted-foreground">
                {t('sizing.peak', { value: formatBytes(memPeak) })}
              </dd>
              {memHeadroom !== 'unknown' ? (
                <dd className="mt-1 text-xs text-foreground">
                  {t(`sizing.headroom.${memHeadroom}`)}
                </dd>
              ) : null}
            </div>
            <div>
              <dt className="text-xs text-muted-foreground uppercase">
                {t('sizing.cpu')}
              </dt>
              <dd className="mt-1 text-sm text-foreground">
                {t('sizing.usedOfLimit', {
                  used: `${(cpuNow ?? 0).toFixed(1)}%`,
                  limit:
                    cpuLimit > 0
                      ? formatNanoCpus(cpuLimit)
                      : t('sizing.noLimit'),
                })}
              </dd>
              <dd className="text-xs text-muted-foreground">
                {t('sizing.peak', { value: `${(cpuPeak ?? 0).toFixed(1)}%` })}
              </dd>
            </div>
          </dl>
        )}
      </CardContent>
    </Card>
  )
}
