import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RocketGlassIcon } from '@thesvg/ui-react'
import { useIsRoot } from '../../hooks/useIsRoot'
import { guideStep, isOpen, showGuideEntryPoints } from '../../lib/setupGuide'
import type { SetupGuideStepId } from '../../lib/setupGuide'
import { useSetupGuide } from '../../queries/setupGuide'

// One quiet line on a page that belongs to an unfinished setup step.
export function SetupStepHint({ step }: { step: SetupGuideStepId }) {
  const { t } = useTranslation('getStarted')
  const isRoot = useIsRoot()
  const { data: guide } = useSetupGuide()
  const current = guideStep(guide, step)
  if (!isRoot || !showGuideEntryPoints(guide) || !current || !isOpen(current)) {
    return null
  }
  return (
    <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
      <RocketGlassIcon aria-hidden="true" className="size-4.5" />
      {t('hint.line', { step: t(`steps.${step}.title`) })}
      <Link
        to="/get-started"
        className="font-medium text-foreground hover:underline"
      >
        {t('hint.cta')}
      </Link>
    </p>
  )
}
