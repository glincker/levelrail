import { useCallback, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { buildZip, type DropSelection } from '../lib/zipSite'
import { appKeys } from '../queries/apps'
import { deployAttemptKeys } from '../queries/deployAttempts'
import { deployKeys } from '../queries/deploys'
import {
  publishDroppedSite,
  type DropStage,
  type PublishDroppedSiteResult,
} from '../queries/dropDeploy'
import { staticSitesKeys } from '../queries/staticSites'

export type DropPhase =
  | { state: 'idle' }
  | { state: 'busy'; stage: DropStage | 'packing'; progress: number }
  | { state: 'error'; error: unknown }
  | { state: 'done'; result: PublishDroppedSiteResult }

/** Runs one drop deploy: pack a folder into a zip, upload, publish, and refresh the cached lists. */
export function useDropPublish(
  onDone?: (appName: string, r: PublishDroppedSiteResult) => void,
) {
  const queryClient = useQueryClient()
  const [phase, setPhase] = useState<DropPhase>({ state: 'idle' })

  const reset = useCallback(() => setPhase({ state: 'idle' }), [])

  const run = useCallback(
    async (selection: DropSelection, appName: string, create: boolean) => {
      setPhase({ state: 'busy', stage: 'packing', progress: 0 })
      try {
        const zip =
          selection.kind === 'zip'
            ? selection.file
            : await buildZip(selection.files)
        const result = await publishDroppedSite({
          appName,
          create,
          zip,
          onStage: (stage) => setPhase({ state: 'busy', stage, progress: 0 }),
          onProgress: (progress) =>
            setPhase({ state: 'busy', stage: 'uploading', progress }),
        })
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: appKeys.list() }),
          queryClient.invalidateQueries({ queryKey: staticSitesKeys.all }),
          queryClient.invalidateQueries({
            queryKey: deployKeys.status(appName),
          }),
          queryClient.invalidateQueries({
            queryKey: deployAttemptKeys.list(appName),
          }),
        ])
        setPhase({ state: 'done', result })
        onDone?.(appName, result)
      } catch (error) {
        setPhase({ state: 'error', error })
      }
    },
    [onDone, queryClient],
  )

  return { phase, run, reset }
}
