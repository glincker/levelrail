import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { WarningCircleIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import type { useDockerDegraded } from './useDockerDegraded'

export function DockerDegradedBanner({
  state,
}: {
  state: ReturnType<typeof useDockerDegraded>
}) {
  const { t } = useTranslation('images')
  if (!state.visible) return null
  return (
    <div
      role="alert"
      className="flex h-9 shrink-0 items-center gap-2 border-b border-destructive/20 bg-destructive/10 px-4 text-sm text-destructive"
    >
      <span aria-hidden="true" className="[&_svg]:size-4">
        <WarningCircleIcon />
      </span>
      <span className="min-w-0 flex-1 truncate">
        {t('dockerDegraded.message')}
      </span>
      <Button size="sm" variant="ghost" render={<Link to="/images" />}>
        {t('dockerDegraded.details')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={state.checking}
        onClick={state.recheck}
      >
        {state.checking
          ? t('dockerDegraded.rechecking')
          : t('dockerDegraded.recheck')}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={t('dockerDegraded.dismiss')}
        title={t('dockerDegraded.dismiss')}
        onClick={state.dismiss}
      >
        <XIcon />
      </Button>
    </div>
  )
}
