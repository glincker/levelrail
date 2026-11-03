import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { firstAppProgress, showGuideEntryPoints } from '../../lib/setupGuide'
import { useSetGuideHidden, useSetupGuide } from '../../queries/setupGuide'

export function SetupChipView({
  percent,
  progress,
  onHide,
}: {
  percent: number
  progress?: { done: number; total: number } | null
  onHide: () => void
}) {
  const { t } = useTranslation('getStarted')
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-border bg-card py-0.5 pr-1 pl-2.5 text-xs">
      <span className="tabular-nums text-muted-foreground">
        {progress ? t('chip.firstApp', progress) : t('chip.label', { percent })}
      </span>
      <Link
        to="/get-started"
        className="inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 font-medium text-foreground hover:bg-muted"
      >
        {t('chip.cta')}
        <ArrowRightGlassIcon className="size-4.5" aria-hidden="true" />
      </Link>
      <button
        type="button"
        aria-label={t('actions.hide')}
        onClick={onHide}
        className="rounded-full p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        <XGlassIcon className="size-4.5" />
      </button>
    </span>
  )
}

export function SetupChip() {
  const { data: guide } = useSetupGuide()
  const hide = useSetGuideHidden()
  if (!guide || !showGuideEntryPoints(guide)) return null
  return (
    <SetupChipView
      percent={guide.ready_percent}
      progress={firstAppProgress(guide)}
      onHide={() => {
        hide.mutate(true)
      }}
    />
  )
}
