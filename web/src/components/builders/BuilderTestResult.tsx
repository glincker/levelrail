import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
import { formatBytes } from '../../lib/format'
import { osArchLabel } from '../../lib/builderFormat'
import type { ApiError } from '../../lib/apiError'
import type { BuilderTestResult as Result } from '../../queries/builders'

// Inline outcome of a builder probe; the page stays usable while it runs.
export function BuilderTestOutcome({
  pending,
  result,
  error,
}: Readonly<{
  pending: boolean
  result?: Result
  error?: ApiError | null
}>) {
  const { t } = useTranslation('nodes')
  if (pending) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm">
        <LoaderCircleGlassIcon
          aria-hidden="true"
          size="1.4em"
          className="animate-spin motion-reduce:animate-none"
        />
        {t('builders.detail.testing')}
      </p>
    )
  }
  if (error) {
    return (
      <p role="alert" className="text-sm text-tone-danger">
        {t('builders.detail.testRequestFailed', { message: error.message })}
      </p>
    )
  }
  if (!result) return null
  const facts = [
    result.ok
      ? t('builders.detail.latency', { ms: result.latency_ms })
      : (result.error ?? ''),
    osArchLabel(result.os, result.arch),
    result.disk_free_bytes === undefined
      ? ''
      : t('builders.detail.freeDisk', {
          value: formatBytes(result.disk_free_bytes),
        }),
    result.buildkit_version
      ? t('builders.detail.buildkit', { value: result.buildkit_version })
      : '',
  ].filter(Boolean)
  return (
    <p
      role="status"
      className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm"
    >
      {result.ok ? (
        <span className="flex items-center gap-1.5 font-medium text-tone-success">
          <CircleCheckGlassIcon aria-hidden="true" size="1.4em" />
          {t('builders.detail.testPass')}
        </span>
      ) : (
        <span className="flex items-center gap-1.5 font-medium text-tone-danger">
          <CircleXGlassIcon aria-hidden="true" size="1.4em" />
          {t('builders.detail.testFail')}
        </span>
      )}
      {facts.map((f) => (
        <span key={f} className="text-muted-foreground">
          {f}
        </span>
      ))}
    </p>
  )
}
