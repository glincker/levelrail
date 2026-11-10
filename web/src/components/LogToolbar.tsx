import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CopyGlassIcon,
  DownloadGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { LogFilter, LogLevelFilter } from '../lib/logFilter'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { SegmentedControl } from './ui/segmented-control'

const LEVELS: readonly LogLevelFilter[] = [
  'all',
  'errors',
  'warnings',
  'info',
  'debug',
]

export function LogToolbar({
  filter,
  onFilterChange,
  shown,
  total,
  onCopy,
  onDownload,
  onJumpToError,
}: {
  filter: LogFilter
  onFilterChange: (next: LogFilter) => void
  shown: number
  total: number
  onCopy: () => Promise<void>
  onDownload: () => void
  onJumpToError: () => void
}) {
  const { t } = useTranslation('uiShared')
  const activeLevel = filter.level ?? 'all'
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    await onCopy()
    setCopied(true)
    window.setTimeout(() => {
      setCopied(false)
    }, 1500)
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative min-w-48 flex-1">
        <SearchGlassIcon
          className="absolute top-1/2 left-2 size-5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          value={filter.text}
          onChange={(e) => {
            onFilterChange({ ...filter, text: e.target.value })
          }}
          placeholder={t('logToolbar.filterPlaceholder')}
          aria-label={t('logToolbar.filterLabel')}
          className="h-7 pl-9 text-xs"
        />
      </div>
      <SegmentedControl<LogLevelFilter>
        aria-label={t('logToolbar.level')}
        value={activeLevel}
        options={LEVELS.map((value) => ({
          value,
          label: t(`logToolbar.levels.${value}`),
        }))}
        onValueChange={(level) => {
          onFilterChange({ ...filter, stderrOnly: false, level })
        }}
      />
      <Button type="button" size="xs" variant="outline" onClick={onJumpToError}>
        <CircleAlertGlassIcon
          className="text-tone-danger"
          size="1.4em"
          aria-hidden="true"
        />
        {t('logToolbar.jumpToError')}
      </Button>
      <Button
        type="button"
        size="xs"
        variant="outline"
        onClick={() => void handleCopy()}
      >
        {copied ? (
          <CheckGlassIcon size="1.4em" aria-hidden="true" />
        ) : (
          <CopyGlassIcon size="1.4em" aria-hidden="true" />
        )}
        {copied ? t('logToolbar.copied') : t('logToolbar.copy')}
      </Button>
      <Button type="button" size="xs" variant="outline" onClick={onDownload}>
        <DownloadGlassIcon size="1.4em" aria-hidden="true" />
        {t('logToolbar.download')}
      </Button>
      <span
        role="status"
        className={
          shown !== total ? 'text-xs text-muted-foreground' : 'sr-only'
        }
      >
        {shown !== total
          ? t('logToolbar.shownOf', {
              shown: shown.toLocaleString(),
              total: total.toLocaleString(),
            })
          : copied
            ? t('logToolbar.copiedAnnounce')
            : ''}
      </span>
    </div>
  )
}
