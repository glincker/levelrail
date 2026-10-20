import { useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { InfoIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'

export interface InlineHelpProps {
  summary: string
  children?: ReactNode
  learnMoreHref?: string
  defaultOpen?: boolean
  className?: string
}

/** Always-true explanations: one line visible, the detail behind "Why?". */
export function InlineHelp({
  summary,
  children,
  learnMoreHref,
  defaultOpen = false,
  className,
}: Readonly<InlineHelpProps>) {
  const { t } = useTranslation('traffic')
  const [open, setOpen] = useState(defaultOpen)
  const detailId = useId()
  const hasDetail = Boolean(children) || Boolean(learnMoreHref)

  return (
    <div className={cn('text-xs text-muted-foreground', className)}>
      <p className="flex flex-wrap items-center gap-1.5">
        <InfoIcon aria-hidden="true" className="size-3.5 shrink-0" />
        <span>{summary}</span>
        {hasDetail ? (
          <button
            type="button"
            aria-expanded={open}
            aria-controls={detailId}
            onClick={() => setOpen((v) => !v)}
            className="rounded-sm font-medium text-[var(--brand-accent)] underline-offset-2 outline-none hover:underline focus-visible:ring-3 focus-visible:ring-ring/50 dark:text-[var(--brand-accent-dark)]"
          >
            {t('help.why')}
          </button>
        ) : null}
      </p>
      {hasDetail && open ? (
        <div id={detailId} className="mt-1.5 space-y-1.5 pl-5">
          {children}
          {learnMoreHref ? (
            <a
              href={learnMoreHref}
              target="_blank"
              rel="noreferrer"
              className="font-medium text-[var(--brand-accent)] underline-offset-2 hover:underline dark:text-[var(--brand-accent-dark)]"
            >
              {t('help.learnMore')}
            </a>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
