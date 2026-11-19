import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { AppStatusSummary } from '../../types/appDetail'
import { failureReason } from '../../lib/failureReason'
import { fixTarget, type FixTarget } from '../../lib/appFailureFix'

const LABEL_KEY: Record<FixTarget, string> = {
  deploys: 'failing.fixLabel',
  health: 'failing.fixLabelHealth',
  domains: 'failing.fixLabelDomains',
  pages: 'failing.fixLabelPages',
}

const FIX_CLASS =
  'relative z-10 text-xs font-medium text-primary underline underline-offset-2 hover:no-underline focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none'

export function FixLink({
  name,
  target,
}: Readonly<{ name: string; target: FixTarget }>) {
  const { t } = useTranslation('appRows')
  const label = t(LABEL_KEY[target], { name })
  const text = t('failing.fix')
  switch (target) {
    case 'pages':
      return (
        <Link
          to="/pages/$name"
          params={{ name }}
          aria-label={label}
          className={FIX_CLASS}
        >
          {text}
        </Link>
      )
    case 'health':
      return (
        <Link
          to="/apps/$name/health"
          params={{ name }}
          aria-label={label}
          className={FIX_CLASS}
        >
          {text}
        </Link>
      )
    case 'domains':
      return (
        <Link
          to="/apps/$name/domains"
          params={{ name }}
          aria-label={label}
          className={FIX_CLASS}
        >
          {text}
        </Link>
      )
    case 'deploys':
      return (
        <Link
          to="/apps/$name/deploys"
          params={{ name }}
          aria-label={label}
          className={FIX_CLASS}
        >
          {text}
        </Link>
      )
  }
}

/** The failure reason (first line, full text in the title) plus a Fix link; nothing unless failing. */
export function FailureReason({
  status,
  name,
  isStatic = false,
}: Readonly<{
  status: AppStatusSummary
  name: string
  isStatic?: boolean
}>) {
  const { t } = useTranslation('appRows')
  if (status.variant !== 'destructive') return null
  const text = failureReason(status) ?? t('failing.fallback')
  return (
    <>
      <span
        className="max-w-full min-w-0 truncate text-xs text-muted-foreground"
        title={text}
      >
        {text.split('\n')[0]}
      </span>
      <FixLink name={name} target={fixTarget(status, isStatic)} />
    </>
  )
}
