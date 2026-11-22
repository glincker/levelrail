import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { HelpLink } from '@/components/HelpLink'
import { cn } from '@/lib/utils'

// Docs page per term, resolved by HelpLink (it renders nothing when the page is unknown).
export const TERM_HELP_PATHS = {
  ingress: '/domains-and-ingress',
  replica: '/autoscaling',
  volume: '/architecture',
  cert: '/domains',
  aRecord: '/dns',
  reconciler: '/architecture',
  agent: '/architecture',
} as const

export type TermKey = keyof typeof TERM_HELP_PATHS

export interface TermProps {
  term: TermKey
  /** Visible text; defaults to the glossary label for the term. */
  children?: ReactNode
  className?: string
}

// Hover opens after a short delay, tap or Enter toggles, so it works on touch too.
export function Term({ term, children, className }: TermProps) {
  const { t } = useTranslation('glossary', { useSuspense: false })
  const label = children ?? t(`terms.${term}.label`)
  return (
    <Popover>
      <PopoverTrigger
        openOnHover
        delay={200}
        className={cn(
          'cursor-help rounded-sm underline decoration-dotted decoration-muted-foreground/60 underline-offset-4 outline-none hover:decoration-foreground focus-visible:ring-2 focus-visible:ring-ring/60',
          className,
        )}
      >
        {label}
      </PopoverTrigger>
      <PopoverContent
        side="top"
        className="w-64 max-w-[calc(100vw-2rem)] rounded-xl p-3 text-xs font-normal normal-case leading-relaxed shadow-floating"
      >
        <p>{t(`terms.${term}.body`)}</p>
        <HelpLink
          path={TERM_HELP_PATHS[term]}
          label={t('learnMore')}
          variant="inline"
          className="mt-2"
        />
      </PopoverContent>
    </Popover>
  )
}
