import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  CaretDownIcon,
  HammerIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
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
        <ArrowsClockwiseIcon aria-hidden="true" />
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
            <CaretDownIcon aria-hidden="true" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={onRebuild}>
              <HammerIcon aria-hidden="true" />
              {t('deployRetry.rebuild')}
            </DropdownMenuItem>
            {noCacheSupported ? (
              <DropdownMenuItem onClick={onRebuildNoCache}>
                <TrashIcon aria-hidden="true" />
                {t('deployRetry.rebuildNoCache')}
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  )
}
