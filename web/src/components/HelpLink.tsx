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
import { splitHash } from '../lib/docsPaths'
import docsPathIndex from 'virtual:docs-path-index'

interface HelpLinkProps {
  /**
   * Path relative to a docs site root, e.g. "/troubleshooting" or
   * "/security#fresh-box-hardening-checklist". Must point at a real
   * page; never invent one.
   */
  path: string
  /** Accessible name, also the tooltip text (icon variant) or link text (inline variant). */
  label: string
  /** "icon": compact question-mark button with a tooltip. "inline": visible text link. */
  variant?: 'icon' | 'inline'
  className?: string
}

interface ResolvedHelpLink {
  href: string
  external: boolean
}

// Prefers the bundled in-app page (works offline, no brand.DocsURL
// needed) and only falls back to the hosted docs site when this path
// isn't bundled under /docs. Returns null when neither exists, so this
// never renders a link with nothing real behind it.
function resolveHelpLink(
  path: string,
  docsUrl: string,
): ResolvedHelpLink | null {
  const [basePath, hash] = splitHash(path)

  if (docsPathIndex.has(basePath)) {
    return { href: `/help${basePath}${hash}`, external: false }
  }
  if (docsUrl) {
    return { href: `${docsUrl}${path}`, external: true }
  }
  return null
}

// Reusable contextual-help affordance: links to the bundled in-app /help
// page when one exists for this path, otherwise to this instance's
// hosted docs (brand.DocsURL, configurable per APP_BRAND_DOCS_URL, never
// a hardcoded domain), otherwise renders nothing.
export function HelpLink({
  path,
  label,
  variant = 'icon',
  className,
}: HelpLinkProps) {
  const brand = useBrand()
  const resolved = resolveHelpLink(path, brand.DocsURL)
  if (!resolved) return null
  const { href, external } = resolved
  const externalProps = external ? { target: '_blank', rel: 'noreferrer' } : {}

  if (variant === 'inline') {
    return (
      <a
        href={href}
        {...externalProps}
        className={cn(
          'inline-flex items-center gap-1 text-sm text-primary underline underline-offset-4 hover:no-underline',
          className,
        )}
      >
        {label}
        {external ? <ArrowSquareOutIcon className="size-3.5" /> : null}
      </a>
    )
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <a
            href={href}
            {...externalProps}
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
