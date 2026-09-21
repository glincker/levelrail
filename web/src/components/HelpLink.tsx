import {
  ArrowSquareOutIcon,
  QuestionIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useBrand } from '../hooks/useBrand'

interface HelpLinkProps {
  /**
   * Path relative to this instance's docs site root (brand.DocsURL), e.g.
   * "/troubleshooting" or "/security#fresh-box-hardening-checklist". Must
   * point at a real page; never invent one.
   */
  path: string
  /** Accessible name, also the tooltip text (icon variant) or link text (inline variant). */
  label: string
  /** "icon": compact question-mark button with a tooltip. "inline": visible text link. */
  variant?: 'icon' | 'inline'
  className?: string
}

// Reusable contextual-help affordance for linking out to this instance's
// own hosted docs (brand.DocsURL, configurable per APP_BRAND_DOCS_URL,
// never a hardcoded domain). Renders nothing when no docs site is
// configured, the same "no invented URL" rule AppSidebar/tokens.tsx/
// general.tsx already follow for their own brand.DocsURL links.
export function HelpLink({
  path,
  label,
  variant = 'icon',
  className,
}: HelpLinkProps) {
  const brand = useBrand()
  if (!brand.DocsURL) return null
  const href = `${brand.DocsURL}${path}`

  if (variant === 'inline') {
    return (
      <a
        href={href}
        target="_blank"
        rel="noreferrer"
        className={cn(
          'inline-flex items-center gap-1 text-sm text-primary underline underline-offset-4 hover:no-underline',
          className,
        )}
      >
        {label}
        <ArrowSquareOutIcon className="size-3.5" />
      </a>
    )
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <a
            href={href}
            target="_blank"
            rel="noreferrer"
            aria-label={label}
            className={cn(
              buttonVariants({ variant: 'ghost', size: 'icon-xs' }),
              'text-muted-foreground hover:text-foreground',
              className,
            )}
          >
            <QuestionIcon />
          </a>
        }
      />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
