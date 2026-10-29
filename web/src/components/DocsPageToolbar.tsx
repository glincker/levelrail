import { useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BookmarkGlassIcon,
  CheckGlassIcon,
  CopyGlassIcon,
  DownloadGlassIcon,
  PinGlassIcon,
  SocialGithubGlassIcon,
} from '@thesvg/ui-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button, buttonVariants } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBrand } from '@/hooks/useBrand'
import { usePinnedPages } from '@/hooks/usePinnedPages'
import { routePathToDocsSiteUrl, routePathWithHash } from '@/lib/docsPathMapper'
import { cn } from '@/lib/utils'

interface DocsPageToolbarProps {
  /** Raw markdown source of the current page, for "Copy as Markdown". */
  markdown: string
  /** Doc-relative source path, e.g. "getting-started.md". */
  file: string
  /** In-app route path, e.g. "/getting-started". */
  routePath: string
  /** Page title for pinning. */
  pageTitle: string
}

export function DocsPageToolbar({
  markdown,
  file,
  routePath,
  pageTitle,
}: DocsPageToolbarProps) {
  const { t } = useTranslation('help')
  const brand = useBrand()
  const [copied, setCopied] = useState(false)
  const { isPinned, toggle } = usePinnedPages()

  const githubHref = brand.RepoURL
    ? `${brand.RepoURL}/blob/main/docs/${file}`
    : null
  const currentHash = typeof window !== 'undefined' ? window.location.hash : ''
  const docsHref = brand.DocsURL
    ? routePathToDocsSiteUrl(
        routePathWithHash(routePath, currentHash.slice(1)),
        brand.DocsURL,
      )
    : null

  const pinned = isPinned(routePath)

  const handleCopy = useCallback(() => {
    navigator.clipboard
      .writeText(markdown)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
        toast.add({
          title: t('toolbar.copyMarkdown'),
          type: 'success',
        })
      })
      .catch(() => {
        toast.add({
          title: 'Could not copy to clipboard',
          type: 'error',
        })
      })
  }, [markdown, t])

  return (
    <div className="mb-4 flex justify-end gap-2 flex-wrap">
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => toggle(routePath, pageTitle)}
        title={pinned ? t('unpin.tooltip') : t('pin.tooltip')}
        className="gap-1.5 text-muted-foreground"
      >
        {pinned ? (
          <PinGlassIcon className="size-5" />
        ) : (
          <PinGlassIcon className="size-5" />
        )}
        {pinned ? t('unpin.label') : t('pin.label')}
      </Button>
      {docsHref && (
        <a
          href={docsHref}
          target="_blank"
          rel="noopener noreferrer"
          className={cn(buttonVariants({ size: 'sm' }), 'gap-1.5')}
        >
          <BookmarkGlassIcon className="size-5" />
          {t('toolbar.openDocsSite')}
        </a>
      )}
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
              <DownloadGlassIcon className="size-5" />
              {t('toolbar.export')}
            </button>
          }
        />
        <DropdownMenuContent>
          <DropdownMenuItem onClick={handleCopy}>
            {copied ? (
              <CheckGlassIcon size="1.4em" />
            ) : (
              <CopyGlassIcon size="1.4em" />
            )}
            {t('toolbar.copyMarkdown')}
          </DropdownMenuItem>
          {githubHref ? (
            <DropdownMenuLinkItem
              href={githubHref}
              target="_blank"
              rel="noreferrer"
            >
              <SocialGithubGlassIcon size="1.4em" />
              {t('toolbar.viewSourceGitHub')}
            </DropdownMenuLinkItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
