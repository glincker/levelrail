import { Link } from '@tanstack/react-router'
import {
  ArrowSquareOutIcon,
  RocketLaunchIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { useRedeployApp } from '../../hooks/useRedeployApp'
import type { AppListEntry } from '../../types/appDetail'

export function AppQuickActions({ app }: { app: AppListEntry }) {
  const { redeploy, isPending } = useRedeployApp(app.name, app.image)
  const domain = app.domains?.[0]
  return (
    <span className="relative z-10 flex items-center gap-0.5 opacity-0 transition-opacity duration-150 group-focus-within/row:opacity-100 group-hover/row:opacity-100 motion-reduce:transition-none">
      {domain ? (
        <Button
          size="icon-sm"
          variant="ghost"
          aria-label={`Open ${domain}`}
          render={
            <a href={`https://${domain}`} target="_blank" rel="noreferrer" />
          }
          nativeButton={false}
        >
          <ArrowSquareOutIcon />
        </Button>
      ) : null}
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={`Redeploy ${app.name}`}
        disabled={isPending}
        onClick={redeploy}
      >
        <RocketLaunchIcon />
      </Button>
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={`Logs for ${app.name}`}
        render={<Link to="/apps/$name/logs" params={{ name: app.name }} />}
        nativeButton={false}
      >
        <TerminalWindowIcon />
      </Button>
    </span>
  )
}
