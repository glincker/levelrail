import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

const LINES = ['deploy', 'protect', 'time'] as const

/** WelcomeStep says what the product does and how long setup takes; the footer carries Start and Skip. */
export function WelcomeStep({
  busy,
  onDismiss,
}: {
  busy: boolean
  onDismiss: () => void
}) {
  const { t } = useTranslation('onboarding')
  return (
    <div className="space-y-4">
      <ul className="space-y-2.5">
        {LINES.map((line) => (
          <li key={line} className="flex items-start gap-2.5 text-sm">
            <CircleCheckGlassIcon
              aria-hidden="true"
              className="mt-0.5 size-4.5 shrink-0 text-tone-success"
            />
            {t(`welcome.${line}`)}
          </li>
        ))}
      </ul>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={busy}
        onClick={onDismiss}
      >
        {t('welcome.dismiss')}
      </Button>
    </div>
  )
}
