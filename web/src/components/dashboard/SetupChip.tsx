import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRightIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { showGuideEntryPoints } from '../../lib/setupGuide'
import { useSetGuideHidden, useSetupGuide } from '../../queries/setupGuide'

export function SetupChipView({
  percent,
  onHide,
}: {
  percent: number
  onHide: () => void
}) {
  const { t } = useTranslation('getStarted')
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-border bg-card py-0.5 pr-1 pl-2.5 text-xs">
      <span className="tabular-nums text-muted-foreground">
        {t('chip.label', { percent })}
      </span>
      <Link
        to="/get-started"
        className="inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 font-medium text-foreground hover:bg-muted"
      >
        {t('chip.cta')}
        <ArrowRightIcon className="size-3" aria-hidden="true" />
      </Link>
      <button
        type="button"
        aria-label={t('actions.hide')}
        onClick={onHide}
        className="rounded-full p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
      >
        <XIcon className="size-3" />
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
      onHide={() => {
        hide.mutate(true)
      }}
    />
  )
}
