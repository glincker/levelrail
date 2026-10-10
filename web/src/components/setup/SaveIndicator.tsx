import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleNotchIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'

/** SaveIndicator tells the operator their progress is persisted, so leaving the wizard is safe. */
export function SaveIndicator({
  saving,
  errorMessage,
}: {
  saving: boolean
  errorMessage?: string
}) {
  const { t } = useTranslation('setup')
  let content = (
    <>
      <CheckCircleIcon
        className="size-4 text-tone-success"
        aria-hidden="true"
      />
      {t('shell.saved')}
    </>
  )
  if (saving) {
    content = (
      <>
        <CircleNotchIcon
          className="size-4 animate-spin motion-reduce:animate-none"
          aria-hidden="true"
        />
        {t('shell.saving')}
      </>
    )
  } else if (errorMessage) {
    content = (
      <>
        <WarningCircleIcon
          className="size-4 text-destructive"
          aria-hidden="true"
        />
        {t('shell.saveError', { message: errorMessage })}
      </>
    )
  }
  return (
    <p
      role="status"
      className="flex items-start gap-1.5 text-xs text-muted-foreground"
    >
      {content}
    </p>
  )
}
