import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  LayersGlassIcon,
  RotateCwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { useComposeStack, useRedeployCompose } from '../queries/composeStack'
import type { ComposePhase, ComposeStackService } from '../types/compose'
import { formatAge } from '../lib/format'
import { ApiError } from '../lib/apiError'
import { ComposeRollbackDialog, ComposeDownDialog } from './ComposeStackDialogs'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'

type BadgeVariant = 'success' | 'warning' | 'destructive' | 'muted' | 'info'

const PHASE_VARIANT: Record<ComposePhase, BadgeVariant> = {
  running: 'success',
  progressing: 'info',
  degraded: 'warning',
  failed: 'destructive',
  stopped: 'muted',
}

const READY_VARIANT: Record<ComposeStackService['ready'], BadgeVariant> = {
  True: 'success',
  False: 'destructive',
  Unknown: 'info',
}

// Renders only for apps deployed from a compose file; any other app's
// stack lookup is a 404 and the panel stays out of the way.
export function ComposeStackPanel({ appId }: { appId?: string }) {
  const { t } = useTranslation('compose')
  const stack = useComposeStack(appId)
  const redeploy = useRedeployCompose(appId ?? '')
  const columns = useMemo<ResponsiveColumn<ComposeStackService>[]>(
    () => [
      {
        id: 'service',
        primary: true,
        header: t('stack.columns.service'),
        cell: (svc) => (
          <Link
            to="/apps/$name/overview"
            params={{ name: svc.name }}
            className="font-medium hover:underline"
          >
            {svc.key}
          </Link>
        ),
      },
      {
        id: 'status',
        header: t('stack.columns.status'),
        cell: (svc) => (
          <div className="min-w-0">
            <Badge variant={READY_VARIANT[svc.ready]}>
              {t(`stack.ready.${svc.ready}`)}
            </Badge>
            {svc.reason ? (
              <p
                className="mt-1 truncate text-xs text-muted-foreground"
                title={svc.message}
              >
                {svc.reason}
                {svc.message ? `: ${svc.message}` : ''}
              </p>
            ) : null}
          </div>
        ),
      },
      {
        id: 'image',
        header: t('stack.columns.image'),
        priority: 2,
        cell: (svc) => (
          <span className="block truncate font-mono text-xs text-muted-foreground">
            {svc.image}
          </span>
        ),
      },
      {
        id: 'dependsOn',
        header: t('stack.columns.dependsOn'),
        priority: 3,
        cell: (svc) => (
          <span className="text-xs text-muted-foreground">
            {svc.depends_on && svc.depends_on.length > 0
              ? svc.depends_on
                  .map((d) =>
                    svc.depends_on_healthy?.includes(d)
                      ? t('stack.healthyDependency', { name: d })
                      : d,
                  )
                  .join(', ')
              : t('stack.noDependencies')}
          </span>
        ),
      },
    ],
    [t],
  )

  if (!appId || stack.isPending) return null
  if (stack.isError) {
    if (stack.error instanceof ApiError && stack.error.status === 404)
      return null
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
        <AlertDescription>
          {t('stack.loadFailed', { message: stack.error.message })}
        </AlertDescription>
      </Alert>
    )
  }

  const data = stack.data
  const runRedeploy = (service?: string) => {
    redeploy.mutate(service, {
      onSuccess: (res) =>
        toast.add({
          title: t('stack.redeployStarted', {
            services: res.restarted.join(', '),
          }),
          type: 'success',
        }),
      onError: (err) => toast.add({ title: err.message, type: 'error' }),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <LayersGlassIcon className="size-5" aria-hidden="true" />
          {t('stack.title')}
          <Badge variant={PHASE_VARIANT[data.phase]}>
            {t(`stack.phase.${data.phase}`)}
          </Badge>
          {data.revision ? (
            <Badge variant="outline">
              {t('stack.revision', { revision: data.revision })}
            </Badge>
          ) : null}
        </CardTitle>
        <CardDescription>
          {t('stack.description', { count: data.services.length })}
          {data.deployed_at
            ? ` ${t('stack.deployedAt', { when: formatAge(data.deployed_at) })}`
            : null}
          {data.template_id
            ? `, ${t('stack.fromTemplate', { id: data.template_id })}`
            : null}
        </CardDescription>
        <div className="flex flex-wrap gap-2 pt-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={redeploy.isPending}
            onClick={() => {
              runRedeploy()
            }}
          >
            <RotateCwGlassIcon size="1.4em" />
            {t('stack.redeployAll')}
          </Button>
          <ComposeRollbackDialog
            appId={data.app_id}
            currentRevision={data.revision}
          />
          <ComposeDownDialog appId={data.app_id} />
        </div>
      </CardHeader>
      <CardContent>
        <ResponsiveTable
          rows={data.services}
          columns={columns}
          getRowId={(svc) => svc.name}
          ariaLabel={t('stack.tableLabel')}
          renderActions={(svc) => (
            <div className="flex justify-end gap-1">
              <Button
                type="button"
                size="sm"
                variant="ghost"
                render={
                  <Link to="/apps/$name/logs" params={{ name: svc.name }} />
                }
                nativeButton={false}
              >
                {t('stack.logs')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                aria-label={t('stack.redeployServiceLabel', {
                  service: svc.key,
                })}
                disabled={redeploy.isPending}
                onClick={() => {
                  runRedeploy(svc.key)
                }}
              >
                {t('stack.redeployService')}
              </Button>
            </div>
          )}
        />
      </CardContent>
    </Card>
  )
}
