import { useTranslation } from 'react-i18next'
import {
  BroomGlassIcon,
  CircleCheckGlassIcon,
  RefreshCwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  useDockerHealth,
  type DockerHealthCheck,
} from '../../queries/dockerIntegration'
import { sizeLabel } from './imageFormat'

const TONE: Record<
  DockerHealthCheck['status'],
  'success' | 'warning' | 'destructive' | 'muted'
> = {
  ok: 'success',
  warn: 'warning',
  fail: 'destructive',
  unknown: 'muted',
}

/** DockerHealthCard shows engine reachability, version, disk and platform with a Fix button per problem. */
export function DockerHealthCard({
  canFix,
  onFreeSpace,
}: {
  canFix: boolean
  onFreeSpace: () => void
}) {
  const { t } = useTranslation('images')
  const health = useDockerHealth()
  const data = health.data

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle>{t('dockerHealth.title')}</CardTitle>
        <Button
          size="sm"
          variant="outline"
          disabled={health.isFetching}
          onClick={() => {
            void health.refetch()
          }}
        >
          <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
          {t('dockerHealth.recheck')}
        </Button>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {health.isError ? (
          <p className="text-sm text-tone-danger">{health.error.message}</p>
        ) : null}
        {data ? (
          <>
            <p className="text-xs text-muted-foreground">
              {t('dockerHealth.summary', {
                version: data.version ?? '?',
                platform: `${data.os ?? '?'}/${data.arch ?? '?'}`,
                reclaimable: sizeLabel(data.disk_reclaimable_bytes),
                total: sizeLabel(data.disk_total_bytes),
              })}
            </p>
            <ul className="flex flex-col gap-2">
              {data.checks.map((check) => (
                <li
                  key={check.code}
                  className="flex flex-wrap items-center justify-between gap-2 text-sm"
                >
                  <span className="flex items-center gap-2">
                    {check.status === 'ok' ? (
                      <CircleCheckGlassIcon
                        aria-hidden="true"
                        className="size-5 text-tone-success"
                      />
                    ) : (
                      <TriangleAlertGlassIcon
                        aria-hidden="true"
                        className="size-5 text-tone-warning"
                      />
                    )}
                    {check.message}
                  </span>
                  <span className="flex items-center gap-2">
                    <Badge variant={TONE[check.status]}>
                      {t(`dockerHealth.status.${check.status}`)}
                    </Badge>
                    {check.fix_action === 'free_space' && canFix ? (
                      <Button size="sm" onClick={onFreeSpace}>
                        <BroomGlassIcon size="1.4em" aria-hidden="true" />
                        {t('dockerHealth.freeSpace')}
                      </Button>
                    ) : null}
                  </span>
                </li>
              ))}
            </ul>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
