import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon, LayersGlassIcon } from '@thesvg/ui-react'
import { useAppGroup } from '../queries/appGroup'
import { STATUS_DOT_COLOR } from '../lib/appStatus'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  ROW_LINK_OVERLAY,
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import type { AppDetail } from '../types/appDetail'

// A suspense query primed by the route's loader; fetch failures surface
// through the route's errorComponent (routes/apps/$name.tsx), not locally.
export function AppServicesPanel({ appName }: { appName: string }) {
  const { t } = useTranslation('common')
  const { data: group } = useAppGroup(appName)
  const hasDependsOn = group.services.some(
    (svc) => svc.depends_on && svc.depends_on.length > 0,
  )
  const columns = useMemo<ResponsiveColumn<AppDetail>[]>(
    () => [
      {
        id: 'service',
        header: 'Service',
        primary: true,
        cell: (svc) => (
          <span className="relative flex flex-wrap items-center gap-1">
            <Link
              to="/apps/$name/overview"
              params={{ name: svc.name }}
              className={`hover:underline ${ROW_LINK_OVERLAY}`}
            >
              {svc.name}
            </Link>
            {svc.name === appName ? (
              <Badge variant="muted" className="px-1.5 text-[10px]">
                this app
              </Badge>
            ) : null}
          </span>
        ),
      },
      {
        id: 'image',
        header: 'Image',
        priority: 2,
        cell: (svc) => (
          <span className="block truncate font-mono text-xs text-muted-foreground">
            {svc.image}
          </span>
        ),
      },
      {
        id: 'port',
        header: 'Port',
        width: 'w-20',
        cell: (svc) => (
          <span className="text-xs text-muted-foreground">{svc.port}</span>
        ),
      },
      {
        id: 'domains',
        header: 'Domains',
        priority: 2,
        cell: (svc) => (
          <span className="text-xs text-muted-foreground">
            {svc.domains && svc.domains.length > 0 ? (
              <span className="flex items-center gap-1">
                <GlobeGlassIcon
                  className="size-4 shrink-0"
                  aria-hidden="true"
                />
                {svc.domains[0]}
                {svc.domains.length > 1 ? ` +${svc.domains.length - 1}` : ''}
              </span>
            ) : (
              <span className="italic text-muted-foreground/60">no domain</span>
            )}
          </span>
        ),
      },
      ...(hasDependsOn
        ? [
            {
              id: 'dependsOn',
              header: 'Depends on',
              priority: 3 as const,
              cell: (svc: AppDetail) => (
                <span className="text-xs text-muted-foreground">
                  {svc.depends_on && svc.depends_on.length > 0 ? (
                    svc.depends_on.join(', ')
                  ) : (
                    <span className="italic text-muted-foreground/60">
                      none
                    </span>
                  )}
                </span>
              ),
            },
          ]
        : []),
    ],
    [appName, hasDependsOn],
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LayersGlassIcon className="size-4.5" />
          Services
        </CardTitle>
        <CardDescription>
          {group.services.length > 1
            ? `${group.services.length} services deployed together under one app.`
            : 'This app currently has one service. Deploy an app.yaml with multiple services to fan more out under it.'}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-2 text-sm">
          <span
            className={`size-2 rounded-full ${STATUS_DOT_COLOR[group.status.variant]}`}
            aria-hidden="true"
          />
          <span className="text-muted-foreground">Group status:</span>
          <Badge variant={group.status.variant}>{group.status.label}</Badge>
        </div>

        <ResponsiveTable
          rows={group.services}
          columns={columns}
          getRowId={(svc) => svc.name}
          ariaLabel={t('tableLabels.appServices')}
          className="border-0"
        />
      </CardContent>
    </Card>
  )
}
