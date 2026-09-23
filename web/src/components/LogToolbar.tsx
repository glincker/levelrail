import {
  CheckIcon,
  CopyIcon,
  DownloadSimpleIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useState } from 'react'
import type { LogFilter } from '../lib/logFilter'
import { Button } from './ui/button'
import { Input } from './ui/input'

export function LogToolbar({
  filter,
  onFilterChange,
  shown,
  total,
  onCopy,
  onDownload,
}: {
  filter: LogFilter
  onFilterChange: (next: LogFilter) => void
  shown: number
  total: number
  onCopy: () => Promise<void>
  onDownload: () => void
}) {
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
      <Button
        type="button"
        size="xs"
        variant={filter.stderrOnly ? 'default' : 'outline'}
        aria-pressed={filter.stderrOnly}
        onClick={() => {
          onFilterChange({ ...filter, stderrOnly: !filter.stderrOnly })
        }}
      >
        Errors only
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
