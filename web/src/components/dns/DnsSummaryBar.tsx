import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export type CopyFormat = 'zone' | 'table' | 'json'

/** Sticky bar: how many records, live progress, the one primary action and a quiet Copy menu. */
export function DnsSummaryBar({
  count,
  live,
  total,
  checked,
  primary,
  onCopy,
  onCheckAgain,
  checking,
}: {
  count: number
  live: number
  total: number
  /** True once a live check exists, so progress is real and not invented. */
  checked: boolean
  primary: ReactNode
  onCopy: (format: CopyFormat) => void
  onCheckAgain?: () => void
  checking?: boolean
}) {
  const { t } = useTranslation('dnsSetup')
  return (
    <div
      role="region"
      aria-label={t('summary.label')}
      className="sticky top-0 z-10 -mx-3 flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-border bg-card px-3 py-2"
    >
      <div className="min-w-0 flex-1 space-y-1">
        <p className="truncate text-sm font-medium">
          {t('summary.toAdd', { count })}
          {checked && (
            <span
              className="text-muted-foreground"
              aria-live="polite"
              data-testid="live-count"
            >
              {' · '}
              {t('verify.liveCount', { live, total })}
            </span>
          )}
        </p>
        {checked && (
          <progress
            aria-label={t('verify.progress')}
            value={live}
            max={Math.max(total, 1)}
            className="block h-1 w-full appearance-none overflow-hidden rounded-full [&::-moz-progress-bar]:bg-tone-success [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:bg-tone-success [&::-webkit-progress-value]:transition-all motion-reduce:[&::-webkit-progress-value]:transition-none"
          />
        )}
      </div>
      <div className="flex items-center gap-2">
        {onCheckAgain && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={checking}
            onClick={onCheckAgain}
          >
            {checking ? t('verify.checking') : t('verify.checkAgain')}
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button type="button" size="sm" variant="ghost" />}
          >
            {t('manual.copyAll')}
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem onClick={() => onCopy('zone')}>
              {t('summary.zone')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => onCopy('table')}>
              {t('summary.table')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => onCopy('json')}>
              {t('summary.json')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        {primary}
      </div>
    </div>
  )
}
