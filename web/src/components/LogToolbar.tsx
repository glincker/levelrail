import {
  CheckIcon,
  CopyIcon,
  DownloadSimpleIcon,
  MagnifyingGlassIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useState } from 'react'
import type { LogFilter, LogLevelFilter } from '../lib/logFilter'
import { Button } from './ui/button'
import { Input } from './ui/input'

const LEVEL_CHIPS: { value: LogLevelFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'errors', label: 'Errors' },
  { value: 'warnings', label: 'Warnings' },
  { value: 'info', label: 'Info' },
  { value: 'debug', label: 'Debug' },
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
        <MagnifyingGlassIcon
          className="absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          value={filter.text}
          onChange={(e) => {
            onFilterChange({ ...filter, text: e.target.value })
          }}
          placeholder="Filter lines"
          aria-label="Filter log lines"
          className="h-7 pl-7 text-xs"
        />
      </div>
      <div className="flex items-center gap-1" role="group" aria-label="Level">
        {LEVEL_CHIPS.map((chip) => (
          <Button
            key={chip.value}
            type="button"
            size="xs"
            variant={activeLevel === chip.value ? 'default' : 'outline'}
            aria-pressed={activeLevel === chip.value}
            onClick={() => {
              onFilterChange({
                ...filter,
                stderrOnly: false,
                level: chip.value,
              })
            }}
          >
            {chip.label}
          </Button>
        ))}
      </div>
      <Button type="button" size="xs" variant="outline" onClick={onJumpToError}>
        <WarningCircleIcon aria-hidden="true" />
        Jump to first error
      </Button>
      <Button
        type="button"
        size="xs"
        variant="outline"
        onClick={() => void handleCopy()}
      >
        {copied ? (
          <CheckIcon aria-hidden="true" />
        ) : (
          <CopyIcon aria-hidden="true" />
        )}
        {copied ? 'Copied' : 'Copy'}
      </Button>
      <Button type="button" size="xs" variant="outline" onClick={onDownload}>
        <DownloadSimpleIcon aria-hidden="true" />
        Download
      </Button>
      {shown !== total ? (
        <span className="text-xs text-muted-foreground" aria-live="polite">
          {shown.toLocaleString()} of {total.toLocaleString()} lines
        </span>
      ) : null}
    </div>
  )
}
