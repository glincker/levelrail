import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  pickNextStep,
  readNextStepDismissed,
  writeNextStepDismissed,
} from '../../lib/nextStepBanner'
import { useSetupGuide } from '../../queries/setupGuide'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'

// One suggestion at a time after a successful deploy: add a domain, take a
// backup, set up alerts, invite a teammate. Dismissing moves to the next.
export function NextStepBanner({
  app,
  latest,
}: {
  app: AppDetail
  latest?: DeployAttempt
}) {
  const { t } = useTranslation('appSetup')
  const { data: guide } = useSetupGuide()
  const [dismissed, setDismissed] = useState(() =>
    readNextStepDismissed(app.name),
  )
  const next = pickNextStep({
    appName: app.name,
    hasDomain: (app.domains?.length ?? 0) > 0,
    deploySucceeded: latest?.status === 'succeeded',
    guide,
    dismissed,
  })
  if (!next) return null
  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card px-4 py-3"
    >
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{t(`nextStep.${next.id}.title`)}</p>
        <p className="text-muted-foreground text-xs">
          {t(`nextStep.${next.id}.body`)}
        </p>
      </div>
      <Button render={<Link to={next.href} />} size="sm">
        {t(`nextStep.${next.id}.cta`)}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={t('nextStep.dismiss')}
        onClick={() => {
          const ids = new Set(dismissed).add(next.id)
          setDismissed(ids)
          writeNextStepDismissed(app.name, ids)
        }}
      >
        <XGlassIcon className="size-4.5" aria-hidden="true" />
      </Button>
    </div>
  )
}
