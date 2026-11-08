import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { RocketGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useDeployGitSource } from '../queries/gitSourceConnect'

// First build after linking a repo: the running version keeps serving until
// this build succeeds, and stays in deploy history for rollback.
export function GitSourceDeployNow({
  appName,
  branch,
}: {
  appName: string
  branch: string
}) {
  const { t } = useTranslation('gitSource')
  const deploy = useDeployGitSource(appName)

  function start() {
    deploy.mutate(branch, {
      onSuccess: (res) => {
        toast.add({
          title: t('connect.deployStarted', { branch: res.branch }),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: t('connect.deployFailed'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-2 rounded-2xl border border-border p-4">
      <p className="text-sm text-muted-foreground">
        {deploy.isSuccess
          ? t('connect.deployRunning')
          : t('connect.deployHint')}
      </p>
      {deploy.isSuccess ? (
        <Button
          size="sm"
          variant="outline"
          nativeButton={false}
          render={<Link to="/apps/$name/deploys" params={{ name: appName }} />}
        >
          {t('connect.viewDeploy')}
        </Button>
      ) : (
        <Button
          type="button"
          size="sm"
          onClick={start}
          disabled={deploy.isPending}
        >
          <RocketGlassIcon className="size-4" />
          {t('connect.deployNow')}
        </Button>
      )}
    </div>
  )
}
