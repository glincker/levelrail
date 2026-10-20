import { useState } from 'react'
import type { ReactNode } from 'react'
import {
  GitBranchIcon,
  RocketLaunchIcon,
  SquaresFourIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { CreateResourceWizard } from '../CreateResourceWizard'
import { useCreateApp } from '../../queries/apps'
import { healthCheckFrom } from '../../lib/healthCheckDefaults'
import { appGate } from '../../lib/setupWizard'
import { ApiError } from '../../lib/apiError'
import { StepFooter } from './StepChrome'
import { FirstAppStatus } from './FirstAppStatus'
import { useFirstAppPolling } from './useSetupPolling'
import { SAMPLE_APP } from './types'
import type { StepProps } from './types'

const HTTP_CONFLICT = 409

/** FirstAppStep deploys a sample, a template, or a repo and waits for it to turn healthy. */
export function FirstAppStep({ onContinue, onSkip, pending }: StepProps) {
  const [trackingSince, setTrackingSince] = useState(() => Date.now())
  const { app, phase } = useFirstAppPolling(trackingSince)
  const createApp = useCreateApp()
  const gate = appGate(phase)

  function deploySample() {
    createApp.mutate(
      {
        name: SAMPLE_APP.name,
        image: SAMPLE_APP.image,
        port: SAMPLE_APP.port,
        health: healthCheckFrom(true, SAMPLE_APP.readinessPath),
      },
      { onSettled: () => setTrackingSince(Date.now()) },
    )
  }

  const sampleExists =
    createApp.error instanceof ApiError &&
    createApp.error.status === HTTP_CONFLICT
  const createError =
    createApp.error && !sampleExists ? createApp.error.message : ''

  return (
    <div className="space-y-4">
      {app ? (
        <FirstAppStatus app={app} phase={phase} />
      ) : (
        <>
          <p className="max-w-prose text-sm text-muted-foreground">
            Deploy something to prove the whole path works: image pull,
            container start, health check, and routing.
          </p>
          <div className="grid gap-3 sm:grid-cols-3">
            <OptionCard
              icon={<RocketLaunchIcon className="size-5 text-primary" />}
              title="Sample app"
              description={`One click. Runs ${SAMPLE_APP.image}, a few megabytes, with a health check on ${SAMPLE_APP.readinessPath}.`}
              action={
                <Button
                  size="sm"
                  onClick={deploySample}
                  disabled={createApp.isPending}
                >
                  {createApp.isPending ? 'Creating...' : 'Deploy sample'}
                </Button>
              }
            />
            <OptionCard
              icon={
                <SquaresFourIcon className="size-5 text-muted-foreground" />
              }
              title="From a template"
              description="Pick a ready-made service from the catalog."
              action={
                <CreateResourceWizard
                  initialSelected="browse-templates"
                  trigger={
                    <Button size="sm" variant="outline">
                      Browse templates
                    </Button>
                  }
                />
              }
            />
            <OptionCard
              icon={<GitBranchIcon className="size-5 text-muted-foreground" />}
              title="Your own repo"
              description="Build from a Dockerfile in a git repository."
              action={
                <CreateResourceWizard
                  initialSelected="dockerfile-git"
                  trigger={
                    <Button size="sm" variant="outline">
                      Deploy a repo
                    </Button>
                  }
                />
              }
            />
          </div>
          {createError ? (
            <p className="text-xs text-destructive">{createError}</p>
          ) : null}
        </>
      )}

      <StepFooter
        gate={gate}
        onContinue={onContinue}
        onSkip={onSkip}
        pending={pending}
      />
    </div>
  )
}

function OptionCard({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode
  title: string
  description: string
  action: ReactNode
}) {
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border p-3">
      <span aria-hidden="true">{icon}</span>
      <p className="text-sm font-medium text-foreground">{title}</p>
      <p className="flex-1 text-xs text-muted-foreground">{description}</p>
      <div>{action}</div>
    </div>
  )
}
