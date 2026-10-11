import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { useTriggerBuild } from '../../queries/builds'
import { isPendingApproval, useTriggerDeploy } from '../../queries/deploys'
import { useQueryClient } from '@tanstack/react-query'
import type { GitSourceResource } from '../../types/gitSource'

interface DeployActionsOptions {
  appName: string
  source?: GitSourceResource
  onDone: () => void
}

// Both actions close the sheet on success and land on live build logs when
// a build was started, the only place a started deploy can be followed.
export function useDeployActions({
  appName,
  source,
  onDone,
}: DeployActionsOptions) {
  const { t } = useTranslation('smartDeploy')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const triggerBuild = useTriggerBuild(appName)
  const triggerDeploy = useTriggerDeploy(appName)

  const deployCommit = (ref: string) => {
    if (!source) return
    triggerBuild.mutate(
      {
        repoUrl: source.repo_url,
        ref,
        buildType: source.build_type,
        buildPath: source.build_path,
        baseDirectory: source.base_directory,
      },
      {
        onSuccess: (result) => {
          void queryClient.invalidateQueries({
            queryKey: ['git-source-commits', appName],
          })
          toast.add({
            title: t('git.toast.started'),
            description: t('git.toast.startedFor', { sha: ref.slice(0, 7) }),
            type: 'success',
          })
          onDone()
          if (result.id) {
            void navigate({
              to: '/apps/$name/deploys/$deployId/logs',
              params: { name: appName, deployId: result.id },
            })
          }
        },
        onError: (error) => {
          toast.add({
            title: t('git.toast.failed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  const deployImage = (
    image: string,
    opts: { confirm?: boolean; pull?: boolean } = {},
  ) => {
    triggerDeploy.mutate(
      { image, confirm: opts.confirm ?? false, pull: opts.pull ?? false },
      {
        onSuccess: (result) => {
          void queryClient.invalidateQueries({
            queryKey: ['git-source-commits', appName],
          })
          toast.add({
            title: isPendingApproval(result)
              ? t('image.toast.approval')
              : t('image.toast.started'),
            type: isPendingApproval(result) ? 'info' : 'success',
          })
          onDone()
        },
        onError: (error) => {
          toast.add({
            title: t('git.toast.failed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return {
    deployCommit,
    deployImage,
    buildPending: triggerBuild.isPending,
    imagePending: triggerDeploy.isPending,
    pending: triggerBuild.isPending || triggerDeploy.isPending,
  }
}
