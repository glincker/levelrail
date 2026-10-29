import { Link } from '@tanstack/react-router'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  InfoGlassIcon,
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { TONE, type Tone } from '../kit/tone'
import { cn } from '@/lib/utils'
import type { DeployAction, DeployState } from '../../lib/deployState'
import { useDeployDetailT } from './useDeployDetailT'

const TONE_ICON: Record<Tone, UiIcon> = {
  neutral: InfoGlassIcon,
  info: LoaderCircleGlassIcon,
  success: CircleCheckGlassIcon,
  warning: TriangleAlertGlassIcon,
  danger: CircleAlertGlassIcon,
  accent: InfoGlassIcon,
}

/** DeployStatusCard is the single authoritative statement of where a deploy is. */
export function DeployStatusCard({
  appName,
  state,
  onOpenBuild,
}: {
  appName: string
  state: DeployState
  onOpenBuild: () => void
}) {
  const { t, msg } = useDeployDetailT()
  const StateIcon = TONE_ICON[state.tone]
  const spinning = state.phase === 'building' || state.phase === 'rolling_out'

  return (
    <section
      aria-label={t('state.title')}
      aria-live="polite"
      className={cn(
        'space-y-3 rounded-lg border p-4',
        TONE[state.tone].border,
        TONE[state.tone].soft,
      )}
    >
      <div className="flex items-start gap-3">
        <StateIcon
          className={cn(
            'mt-0.5 size-5 shrink-0',
            TONE[state.tone].text,
            spinning &&
              !state.stuck &&
              'animate-spin motion-reduce:animate-none',
          )}
          aria-hidden="true"
        />
        <div className="min-w-0 space-y-1">
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {msg(state.label)}
          </p>
          <p className="text-sm text-foreground">{msg(state.headline)}</p>
          {state.next ? (
            <p className="text-sm text-muted-foreground">{msg(state.next)}</p>
          ) : null}
        </div>
      </div>

      {state.disagreement ? (
        <p className="flex items-start gap-2 rounded-md bg-background/60 px-3 py-2 text-sm text-muted-foreground">
          <InfoGlassIcon
            className="mt-0.5 size-5 shrink-0 text-tone-info"
            aria-hidden="true"
          />
          {msg(state.disagreement)}
        </p>
      ) : null}

      {state.stuck && state.stuckCause ? (
        <div className="space-y-2 rounded-md border border-tone-warning-border bg-background/60 px-3 py-2">
          <p className="text-sm font-medium text-foreground">
            {t('state.stuck.title')}
          </p>
          <p className="text-sm text-muted-foreground">
            {msg(state.stuckCause)}
          </p>
        </div>
      ) : null}

      {state.actions.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {state.actions.map((action) => (
            <ActionButton
              key={action}
              action={action}
              appName={appName}
              label={t(`state.action.${action}`)}
              onOpenBuild={onOpenBuild}
            />
          ))}
        </div>
      ) : null}
    </section>
  )
}

function ActionButton({
  action,
  appName,
  label,
  onOpenBuild,
}: {
  action: DeployAction
  appName: string
  label: string
  onOpenBuild: () => void
}) {
  if (action === 'buildTab') {
    return (
      <Button variant="outline" size="sm" onClick={onOpenBuild}>
        {label}
      </Button>
    )
  }
  const link = (() => {
    switch (action) {
      case 'containerLogs':
        return <Link to="/apps/$name/logs" params={{ name: appName }} />
      case 'healthPath':
        return <Link to="/apps/$name/health" params={{ name: appName }} />
      case 'network':
        return <Link to="/apps/$name/network" params={{ name: appName }} />
      case 'redeploy':
        return <Link to="/apps/$name/source" params={{ name: appName }} />
      default:
        return <Link to="/apps/$name/deploys" params={{ name: appName }} />
    }
  })()
  return (
    <Button variant="outline" size="sm" nativeButton={false} render={link}>
      {label}
    </Button>
  )
}
