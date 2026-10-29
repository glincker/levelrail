import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  HammerGlassIcon,
  RefreshCwGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

// Retry reuses the last image; the menu covers the cases where the user
// changed a setting and needs a fresh build, with or without the layer cache.
export function RetrySplitButton({
  onRetry,
  onRebuild,
  onRebuildNoCache,
  pending = false,
  canRebuild,
  noCacheSupported = true,
}: {
  onRetry: () => void
  onRebuild: () => void
  onRebuildNoCache: () => void
  pending?: boolean
  canRebuild: boolean
  // The build API only honours no_cache for Dockerfile builds.
  noCacheSupported?: boolean
}) {
  const { t } = useTranslation('common')
  return (
    <div className="inline-flex items-center">
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={canRebuild ? 'rounded-r-none' : undefined}
        disabled={pending}
        onClick={onRetry}
      >
        <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
        {t('deployRetry.retry')}
      </Button>
      {canRebuild ? (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="-ml-px rounded-l-none px-1.5"
                disabled={pending}
                aria-label={t('deployRetry.more')}
              />
            }
          >
            <ChevronDownGlassIcon size="1.4em" aria-hidden="true" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={onRebuild}>
              <HammerGlassIcon size="1.4em" aria-hidden="true" />
              {t('deployRetry.rebuild')}
            </DropdownMenuItem>
            {noCacheSupported ? (
              <DropdownMenuItem onClick={onRebuildNoCache}>
                <TrashGlassIcon size="1.4em" aria-hidden="true" />
                {t('deployRetry.rebuildNoCache')}
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  )
}
