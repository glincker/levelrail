import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ApiError } from '../../lib/apiError'
import { SAMPLE_APP, firstAppAddress } from '../../lib/firstApp'
import { buildFirstRun } from '../../lib/firstRunSteps'
import { healthCheckFrom } from '../../lib/healthCheckDefaults'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useCreateApp } from '../../queries/apps'
import { appNetworkQueryOptions } from '../../queries/appNetwork'
import { appUrlsQueryOptions } from '../../queries/appUrls'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'
import { systemDoctorQueryOptions } from '../../queries/systemDoctor'
import { FirstRunStrip } from './FirstRunStrip'
import { useFirstAppPolling } from './useFirstAppPolling'

const HTTP_CONFLICT = 409

/** FirstRunStripLive wires the strip to the doctor, HTTPS and app queries and deploys the sample app. */
export function FirstRunStripLive({
  onStarted,
  onDone,
}: {
  onStarted: () => void
  onDone: () => void
}) {
  const isRoot = useIsRoot()
  const createApp = useCreateApp()
  const [mountedAt] = useState(() => Date.now())
  const { app, phase } = useFirstAppPolling(mountedAt)
  const name = app?.name ?? ''
  const { data: network } = useQuery({
    ...appNetworkQueryOptions(name),
    enabled: phase === 'live',
  })
  const { data: urls } = useQuery({
    ...appUrlsQueryOptions(name),
    enabled: phase === 'live',
  })
  const { data: https } = useQuery({
    ...httpsStatusQueryOptions(),
    enabled: isRoot,
    retry: false,
  })
  const { data: doctor } = useQuery({
    ...systemDoctorQueryOptions(),
    enabled: isRoot,
    retry: false,
  })
  const address = app
    ? firstAppAddress(
        app,
        network,
        urls,
        window.location.hostname,
        https?.state === 'issued',
      )?.url
    : undefined
  const model = useMemo(
    () =>
      buildFirstRun({
        doctor,
        https,
        app,
        phase,
        address,
        sampleName: SAMPLE_APP.name,
      }),
    [doctor, https, app, phase, address],
  )

  function deploy() {
    createApp.mutate(
      {
        name: SAMPLE_APP.name,
        image: SAMPLE_APP.image,
        port: SAMPLE_APP.port,
        health: healthCheckFrom(true, SAMPLE_APP.readinessPath),
      },
      {
        onSuccess: onStarted,
        onError: (err) => {
          if (err instanceof ApiError && err.status === HTTP_CONFLICT) {
            onStarted()
          }
        },
      },
    )
  }

  const conflict =
    createApp.error instanceof ApiError &&
    createApp.error.status === HTTP_CONFLICT
  return (
    <FirstRunStrip
      model={model}
      onDeploy={deploy}
      deploying={createApp.isPending}
      error={createApp.error && !conflict ? createApp.error.message : undefined}
      onContinue={model.live ? onDone : undefined}
    />
  )
}
