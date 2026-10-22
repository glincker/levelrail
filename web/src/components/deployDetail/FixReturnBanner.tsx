import { useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { ArrowsClockwiseIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRedeployApp } from '../../hooks/useRedeployApp'
import { clearFixMarker, readFixMarker } from '../../lib/fixReturnMarker'
import { useGitSource } from '../../queries/gitSources'
import { useRebuildApp } from '../../queries/rebuildApp'
import { useDeployDetailT } from './useDeployDetailT'

/** One-click redeploy offered after the user followed a fix link from a failed deploy. */
export function FixReturnBanner({
  appName,
  image,
}: {
  appName: string
  image: string
}) {
  const { t } = useDeployDetailT()
  const [dismissed, setDismissed] = useState(false)
  const onFailedDeployPage = useRouterState({
    select: (s) => s.location.pathname.includes('/deploys/'),
  })
  const { data: gitSource } = useGitSource(appName)
  const rebuild = useRebuildApp(appName)
  const redeploy = useRedeployApp(appName, image, {
    onSuccess: () => {
      clearFixMarker()
      setDismissed(true)
    },
  })

  const marker = dismissed ? null : readFixMarker(appName)
  if (!marker || onFailedDeployPage) return null

  const pending = rebuild.isPending || redeploy.isPending
  const handleRedeploy = () => {
    if (gitSource === undefined) {
      redeploy.redeploy()
      return
    }
    rebuild.mutate(
      { noCache: false },
      {
        onSuccess: () => {
          clearFixMarker()
          setDismissed(true)
          toast.add({ title: t('fixReturn.started'), type: 'success' })
        },
        onError: (error) => {
          toast.add({ title: error.message, type: 'error' })
        },
      },
    )
  }

  return (
    <section
      aria-label={t('fixReturn.region')}
      data-testid="fix-return-banner"
      className="flex flex-wrap items-center gap-3 rounded-lg border border-border bg-muted/40 px-4 py-3"
    >
      <p className="min-w-0 flex-1 text-sm">
        {t('fixReturn.body', { target: t(`fix.link.${marker.labelKey}`) })}
      </p>
      <Button
        type="button"
        size="sm"
        disabled={pending}
        onClick={handleRedeploy}
      >
        <ArrowsClockwiseIcon aria-hidden="true" />
        {t('fixReturn.redeploy')}
      </Button>
      <Link
        to="/apps/$name/deploys/$deployId/logs"
        params={{ name: appName, deployId: marker.deployId }}
        className="text-sm underline underline-offset-2 hover:text-foreground"
      >
        {t('fixReturn.back')}
      </Link>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={t('fixReturn.dismiss')}
        onClick={() => {
          clearFixMarker()
          setDismissed(true)
        }}
      >
        <XIcon aria-hidden="true" />
      </Button>
    </section>
  )
}
