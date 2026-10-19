import { useTranslation } from 'react-i18next'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import type { DeployFailure } from '../types/deployFailure'
import { failureActionFor, isUnknownFailure } from '../lib/failureActions'
import { useRedeployApp } from '../hooks/useRedeployApp'
import { Button } from '@/components/ui/button'

function RetryButton({ appName, image }: { appName: string; image: string }) {
  const { t } = useTranslation('gitBuild')
  const retry = useRedeployApp(appName, image)
  return (
    <Button
      type="button"
      variant="outline"
      size="xs"
      disabled={retry.isPending}
      onClick={retry.redeploy}
    >
      <ArrowsClockwiseIcon aria-hidden="true" />
      {t('failureActions.retry')}
    </Button>
  )
}

/** DeployFailureActions gives every failure code a link to its fix; an unmatched failure also gets Retry. */
export function DeployFailureActions({
  failure,
  appName,
  image,
}: {
  failure: DeployFailure
  appName: string
  image?: string
}) {
  const { t } = useTranslation('gitBuild')
  const action = failureActionFor(failure.code, appName, failure.deploy_id)
  if (!action) return null
  return (
    <span className="mt-1 flex flex-wrap items-center gap-2">
      <a
        href={action.href}
        className="text-xs underline underline-offset-2 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
      >
        {t(`failureActions.${action.labelKey}`)}
      </a>
      {isUnknownFailure(failure.code) && image ? (
        <RetryButton appName={appName} image={image} />
      ) : null}
    </span>
  )
}
