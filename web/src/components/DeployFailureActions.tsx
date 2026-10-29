import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import type { DeployFailure } from '../types/deployFailure'
import {
  failureActionFor,
  fixActionFor,
  isUnknownFailure,
} from '../lib/failureActions'
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
      <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
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
  const fixes = failure.fixes ?? []
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
      {fixes.length > 0 ? (
        <ul
          aria-label={t('failureActions.matchedFixes')}
          className="basis-full space-y-1 text-xs"
        >
          {fixes.map((fix) => {
            const link = fixActionFor(fix.target, appName)
            return (
              <li
                key={`${fix.code}-${fix.kind}`}
                className="flex flex-wrap items-center gap-2"
              >
                <span className="text-muted-foreground">{fix.hint}</span>
                {fix.kind === 'rerun' && image ? (
                  <RetryButton appName={appName} image={image} />
                ) : null}
                {link ? (
                  <a
                    href={link.href}
                    className="underline underline-offset-2 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
                  >
                    {t(`failureActions.${link.labelKey}`)}
                  </a>
                ) : null}
              </li>
            )
          })}
        </ul>
      ) : null}
    </span>
  )
}
