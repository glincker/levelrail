import { Link } from '@tanstack/react-router'
import {
  BellIcon,
  ClockCounterClockwiseIcon,
  PulseIcon,
  ScrollIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Card } from '@/components/ui/card'

// Links into this app's own already-existing sections (same routes and
// icons as AppScopedSidebar.tsx's Observe/Deploy groups), not a parallel
// set of destinations invented for this page.
export function DeployQuickLinks({ appName }: { appName: string }) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <Link to="/apps/$name/logs" params={{ name: appName }} className="block">
        <QuickLinkCard
          icon={ScrollIcon}
          label="Logs"
          description="Live and historical container output"
        />
      </Link>
      <Link
        to="/apps/$name/metrics"
        params={{ name: appName }}
        className="block"
      >
        <QuickLinkCard
          icon={PulseIcon}
          label="Metrics"
          description="CPU, memory, and network usage"
        />
      </Link>
      <Link
        to="/apps/$name/alerts"
        params={{ name: appName }}
        className="block"
      >
        <QuickLinkCard
          icon={BellIcon}
          label="Alerts"
          description="Notification rules for this app"
        />
      </Link>
      <Link
        to="/apps/$name/deploys"
        params={{ name: appName }}
        className="block"
      >
        <QuickLinkCard
          icon={ClockCounterClockwiseIcon}
          label="Deploy history"
          description="Every attempt for this app"
        />
      </Link>
    </div>
  )
}

function QuickLinkCard({
  icon: LinkIcon,
  label,
  description,
}: {
  icon: Icon
  label: string
  description: string
}) {
  return (
    <Card
      size="sm"
      className="h-full gap-1 p-3 transition-colors hover:bg-muted/50"
    >
      <LinkIcon className="size-4 text-muted-foreground" aria-hidden="true" />
      <p className="text-sm font-medium text-foreground">{label}</p>
      <p className="text-xs text-muted-foreground">{description}</p>
    </Card>
  )
}
