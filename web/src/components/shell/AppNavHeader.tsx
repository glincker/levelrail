import { Link } from '@tanstack/react-router'
import { ArrowLeftIcon } from '@phosphor-icons/react/dist/ssr'
import { SidebarMenuButton, useSidebar } from '@/components/ui/sidebar'
import { Sparkline, StatusPill, type Tone } from '@/components/kit'
import { useApp } from '../../queries/apps'
import { useDeployStatus } from '../../queries/deploys'
import { useRequestRateSpark } from './useRequestRateSpark'
import { summarizeAppStatus } from '../../lib/appStatus'

function toneFor(variant: string | null | undefined): Tone {
  if (variant === 'destructive') return 'danger'
  if (variant === 'success') return 'success'
  if (variant === 'warning') return 'warning'
  return 'neutral'
}

export function AppNavHeader({ name }: { name: string }) {
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)
  const { state } = useSidebar()
  const status = summarizeAppStatus(conditions)
  const spark = useRequestRateSpark(name)
  const rail = state === 'collapsed'

  return (
    <div className="flex flex-col gap-1 px-2 pt-1">
      <SidebarMenuButton
        render={<Link to="/apps" />}
        tooltip="Back to Apps"
        size="sm"
      >
        <ArrowLeftIcon />
        <span>All apps</span>
      </SidebarMenuButton>
      {rail ? null : (
        <div className="flex flex-col gap-1.5 rounded-xl bg-sidebar-accent/40 px-3 py-2">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate text-sm font-semibold text-sidebar-foreground">
              {app.name}
            </span>
            <StatusPill
              tone={toneFor(status.variant)}
              label={status.label}
              live={status.label === 'Reconciling'}
              size="sm"
            />
          </div>
          {spark.length > 1 ? (
            <Sparkline
              values={spark}
              width="fill"
              height={20}
              ariaLabel="Request rate, last hour"
            />
          ) : null}
        </div>
      )}
    </div>
  )
}
