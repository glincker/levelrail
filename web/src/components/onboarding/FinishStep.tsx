import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon } from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'

const NEXT = [
  { key: 'deploy', to: '/apps/new' },
  { key: 'guide', to: '/get-started' },
] as const

/** FinishStep is a placeholder until the app and safety steps ship; Finish in the footer ends the flow. */
export function FinishStep() {
  const { t } = useTranslation('onboarding')
  return (
    <ul className="space-y-2">
      {NEXT.map((item) => (
        <li
          key={item.key}
          className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2.5"
        >
          <span className="min-w-0 text-sm">{t(`finish.${item.key}`)}</span>
          <Link
            to={item.to}
            className={buttonVariants({ size: 'sm', variant: 'outline' })}
          >
            {t('finish.open')}
            <ArrowRightGlassIcon aria-hidden="true" />
          </Link>
        </li>
      ))}
    </ul>
  )
}
