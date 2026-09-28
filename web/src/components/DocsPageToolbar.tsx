import { useState } from 'react'
import {
  ArrowSquareOutIcon,
  CheckIcon,
  CopyIcon,
  DownloadSimpleIcon,
  GithubLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { buttonVariants } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBrand } from '@/hooks/useBrand'
import { cn } from '@/lib/utils'

interface DocsPageToolbarProps {
  /** Raw markdown source of the current page, for "Copy as Markdown". */
  markdown: string
  /** Doc-relative source path, e.g. "getting-started.md". */
  file: string
  /** In-app route path, e.g. "/getting-started". */
  routePath: string
}

async function copyMarkdown(markdown: string, onDone: () => void) {
  try {
    await navigator.clipboard.writeText(markdown)
    onDone()
    toast.add({ title: 'Copied page as Markdown', type: 'success' })
  } catch {
    toast.add({ title: 'Could not copy to clipboard', type: 'error' })
  }
}

// Page-level "export" menu: copy the raw source, or jump to the same page
// on GitHub or this instance's hosted docs site. brand.RepoURL and
// brand.DocsURL are both optional (empty means don't render that item),
// the same convention HelpLink already follows.
export function DocsPageToolbar({
  markdown,
  file,
  routePath,
}: DocsPageToolbarProps) {
  const brand = useBrand()
  const [copied, setCopied] = useState(false)

  const githubHref = brand.RepoURL
    ? `${brand.RepoURL}/blob/main/docs/${file}`
    : null
  const hostedHref = brand.DocsURL ? `${brand.DocsURL}${routePath}` : null

  return (
    <div className="mb-4 flex justify-end">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <button
              type="button"
              className={cn(
                buttonVariants({ variant: 'outline', size: 'sm' }),
                'gap-1.5 text-muted-foreground',
              )}
            >
              <DownloadSimpleIcon className="size-3.5" />
              Export
            </button>
          }
        />
        <DropdownMenuContent>
          <DropdownMenuItem
            onClick={() => {
              void copyMarkdown(markdown, () => {
                setCopied(true)
                setTimeout(() => setCopied(false), 2000)
              })
            }}
          >
            {copied ? <CheckIcon /> : <CopyIcon />}
            Copy page as Markdown
          </DropdownMenuItem>
          {githubHref ? (
            <DropdownMenuLinkItem
              href={githubHref}
              target="_blank"
              rel="noreferrer"
            >
              <GithubLogoIcon />
              View source on GitHub
            </DropdownMenuLinkItem>
          ) : null}
          {hostedHref ? (
            <DropdownMenuLinkItem
              href={hostedHref}
              target="_blank"
              rel="noreferrer"
            >
              <ArrowSquareOutIcon />
              Open on {brand.Name} docs
            </DropdownMenuLinkItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
