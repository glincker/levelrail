import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CloudGlassIcon,
  GitBranchGlassIcon,
  GlobeGlassIcon,
  HammerGlassIcon,
  RocketGlassIcon,
  ChevronRightGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { CircleAlertGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import { TONE, type Tone } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import { useAppEdge, type EdgeHealth } from '../../queries/appEdge'
import { CdnSetupDialog } from './CdnSetupDialog'

const HEALTH_TONE: Record<EdgeHealth, Tone> = {
  direct: 'neutral',
  ok: 'success',
  attention: 'warning',
}

function Stage({
  icon: Icon,
  label,
  value,
  tone = 'neutral',
}: {
  icon: UiIcon
  label: string
  value: string
  tone?: Tone
}) {
  return (
    <li className="flex min-w-0 items-center gap-2">
      <Icon
        aria-hidden="true"
        className={cn('size-5 shrink-0', TONE[tone].text)}
      />
      <span className="min-w-0 text-sm">
        <span className="block text-xs text-muted-foreground">{label}</span>
        <span className="block truncate text-foreground">{value}</span>
      </span>
    </li>
  )
}

function deployTone(status: string | undefined): Tone {
  if (status === 'failed' || status === 'error') return 'danger'
  if (status === 'running' || status === 'pending') return 'info'
  return 'success'
}

/** AppEdgeCard shows git, build, deploy, DNS and edge on one line. */
export function AppEdgeCard({ appName }: { appName: string }) {
  const { t } = useTranslation('cdn')
  const edge = useAppEdge(appName)
  const [setupOpen, setSetupOpen] = useState(false)
  const data = edge.data
  if (!data) return null

  const deploy = data.deploy
  const sha = deploy?.commit_sha ? deploy.commit_sha.slice(0, 7) : ''
  const health = data.health
  const HealthIcon =
    health === 'attention' ? CircleAlertGlassIcon : CircleCheckGlassIcon
  const lastPurge = data.last_purge
  const hitRate = data.cache ? Math.round(data.cache.hit_rate * 100) : null

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-semibold text-foreground">
              {t('edge.title')}
            </h2>
            <StatusBadge
              tone={HEALTH_TONE[health]}
              label={t(`edge.health.${health}`)}
              icon={HealthIcon}
            />
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setSetupOpen(true)}
          >
            {t('edge.setup')}
          </Button>
        </div>
        <ol
          aria-label={t('edge.pipelineLabel')}
          className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3 lg:grid-cols-5"
        >
          <Stage
            icon={GitBranchGlassIcon}
            label={t('edge.stage.git')}
            value={sha || deploy?.source || t('edge.noDeploy')}
          />
          <Stage
            icon={HammerGlassIcon}
            label={t('edge.stage.build')}
            value={deploy?.source ?? t('edge.noDeploy')}
          />
          <Stage
            icon={RocketGlassIcon}
            label={t('edge.stage.deploy')}
            value={deploy?.status ?? t('edge.noDeploy')}
            tone={deployTone(deploy?.status)}
          />
          <Stage
            icon={GlobeGlassIcon}
            label={t('edge.stage.dns')}
            value={
              data.dns && data.dns.managed_records > 0
                ? t('edge.dns', { count: data.dns.managed_records })
                : t('edge.dnsNone')
            }
          />
          <Stage
            icon={CloudGlassIcon}
            label={t('edge.stage.edge')}
            value={t(`edge.health.${health}`)}
            tone={HEALTH_TONE[health]}
          />
        </ol>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          {data.provider ? (
            <span>
              {t('edge.provider', {
                provider: data.provider,
                zone: data.zone ?? '-',
                mode: data.ssl_mode ?? '-',
              })}
            </span>
          ) : null}
          {hitRate !== null && data.cache ? (
            <span>
              {t('edge.cache', { rate: hitRate, entries: data.cache.entries })}
            </span>
          ) : null}
          {data.override !== null && data.override !== undefined ? (
            <span>
              {t('edge.override', {
                state: data.override
                  ? t('edge.overrideOn')
                  : t('edge.overrideOff'),
              })}
            </span>
          ) : null}
          {lastPurge ? (
            <span className="inline-flex items-center gap-1">
              <ChevronRightGlassIcon className="size-3.5" aria-hidden="true" />
              {lastPurge.ok
                ? t('edge.lastPurge', {
                    when: new Date(lastPurge.at).toLocaleString(),
                    scope: lastPurge.scope,
                  })
                : t('edge.purgeFailed', { error: lastPurge.error ?? '' })}
            </span>
          ) : null}
        </div>
        {data.ssl_warning || (data.issues && data.issues.length > 0) ? (
          <ul className="space-y-1 text-sm text-tone-warning" role="status">
            {data.ssl_warning ? <li>{data.ssl_warning}</li> : null}
            {(data.issues ?? []).map((issue) => (
              <li key={issue}>{issue}</li>
            ))}
          </ul>
        ) : null}
      </CardContent>
      {setupOpen ? (
        <CdnSetupDialog
          appName={appName}
          open={setupOpen}
          onOpenChange={setSetupOpen}
        />
      ) : null}
    </Card>
  )
}
