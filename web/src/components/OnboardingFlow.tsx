import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  CircleIcon,
  FolderIcon,
  RocketLaunchIcon,
  SparkleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useBrand } from '../hooks/useBrand'
import { useDockerHealthPoll } from '../queries/systemStatus'
import { useCompleteOnboarding } from '../queries/onboarding'
import { CreateResourceWizard } from './CreateResourceWizard'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

// Shown by DashboardOverview in place of its plain WelcomeEmptyState only
// on a genuinely fresh instance: zero apps and onboarding_state.completed
// still false (queries/onboarding.ts). "Skip setup" and deploying a real
// first app both persist that flag server-side, so this never comes back
// once either happens, see DashboardOverview's own comment for the
// "apps.length > 0" half of that contract.
export function OnboardingFlow() {
  const brand = useBrand()
  const { data: status } = useDockerHealthPoll()
  const dockerKnown = status !== undefined
  const dockerConnected = status?.docker_connected ?? true
  const completeOnboarding = useCompleteOnboarding()

  return (
    <Card className="mx-auto max-w-2xl">
      <CardHeader className="flex-row items-start justify-between gap-3 space-y-0">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2 text-lg">
            <SparkleIcon className="size-5 text-primary" />
            Welcome to {brand.Name}
          </CardTitle>
          <CardDescription>
            Three steps to your first running app.
          </CardDescription>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => completeOnboarding.mutate()}
          disabled={completeOnboarding.isPending}
        >
          Skip setup
        </Button>
      </CardHeader>
      <CardContent className="space-y-2.5">
        <OnboardingStep
          done={dockerKnown && dockerConnected}
          title="Docker is reachable"
          description={
            dockerKnown && !dockerConnected
              ? 'This node cannot reach its Docker daemon yet. Deploys will fail until it is back.'
              : 'Your control plane can talk to the Docker daemon.'
          }
          icon={
            dockerKnown && !dockerConnected ? (
              <WarningCircleIcon className="size-5 text-destructive" />
            ) : undefined
          }
          action={
            dockerKnown && !dockerConnected ? (
              <Button variant="outline" size="sm" render={<Link to="/nodes" />}>
                Check node status
              </Button>
            ) : undefined
          }
        />
        <OnboardingStep
          optional
          title="Organize with a project"
          description="Group related apps and databases together. You can always do this later."
          icon={<FolderIcon className="size-5 text-muted-foreground" />}
          action={
            <Button variant="outline" size="sm" render={<Link to="/projects" />}>
              Create a project
            </Button>
          }
        />
        <OnboardingStep
          title="Deploy your first app"
          description="From a git repo, a Docker image, or the built-in template catalog."
          icon={<RocketLaunchIcon className="size-5 text-primary" />}
          action={
            <CreateResourceWizard
              trigger={<Button size="sm">Deploy an app</Button>}
            />
          }
        />
      </CardContent>
    </Card>
  )
}

function OnboardingStep({
  done,
  optional,
  title,
  description,
  icon,
  action,
}: {
  done?: boolean
  optional?: boolean
  title: string
  description: string
  icon?: ReactNode
  action: ReactNode
}) {
  return (
    <div className="flex items-start gap-3 rounded-lg border border-border bg-card/50 p-3">
      <span className="mt-0.5 shrink-0" aria-hidden="true">
        {icon ??
          (done ? (
            <CheckCircleIcon className="size-5 text-green-600 dark:text-green-400" />
          ) : (
            <CircleIcon className="size-5 text-muted-foreground" />
          ))}
      </span>
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="flex items-center gap-2">
          <p className="text-sm font-medium text-foreground">{title}</p>
          {optional ? (
            <Badge variant="outline" className="text-[10px]">
              Optional
            </Badge>
          ) : null}
        </div>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <div className="shrink-0">{action}</div>
    </div>
  )
}
