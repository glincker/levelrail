import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from './apiError'
import { translateShared } from './activeLocale'
import {
  CODE_KIND,
  KEEP_SERVER_TEXT,
  RETRYABLE,
  STATUS_KIND,
  abilityFromMessage,
  isSafeServerText,
  isTimeoutError,
  type ErrorKind,
} from './errorCatalog'

export interface ErrorText {
  title?: string
  message: string
  action?: string
}

type Translate = (key: string, defaultValue: string) => string

function kindOf(err: unknown, raw: string): ErrorKind | undefined {
  if (err instanceof ApiError) {
    if (err.status === 403 && abilityFromMessage(raw)) return 'missingAbility'
    const byCode = err.code ? CODE_KIND[err.code] : undefined
    // A generic status code must not hide a more specific status mapping.
    return (
      byCode ??
      STATUS_KIND[err.status] ??
      (err.status >= 500 ? 'serverError' : undefined)
    )
  }
  if (isTimeoutError(err)) return 'timeout'
  return err instanceof TypeError ? 'network' : undefined
}

function build(err: unknown, tr: Translate): ErrorText {
  const raw = err instanceof Error ? err.message.trim() : ''
  const kind = kindOf(err, raw)
  const safe = isSafeServerText(raw)
  const generic = tr('apiError.generic', 'The server hit an error. Try again.')
  const retry = tr(
    'apiError.friendly.generic.action',
    'Try again, and quote the request ID if it keeps failing.',
  )
  if (!kind) {
    return {
      message: safe ? raw : generic,
      action: safe ? undefined : retry,
    }
  }
  const base = `apiError.friendly.${kind}`
  const ability =
    kind === 'missingAbility' ? abilityFromMessage(raw) : undefined
  const fill = (text: string) =>
    ability ? text.replace('{{ability}}', ability) : text
  const keepServer = KEEP_SERVER_TEXT.has(kind) && safe
  const title = tr(`${base}.title`, '') || undefined
  return {
    title,
    message: keepServer
      ? raw
      : fill(tr(`${base}.message`, safe ? raw : generic)),
    action:
      fill(tr(`${base}.action`, '')) ||
      (RETRYABLE.has(kind) ? retry : undefined),
  }
}

// Translates any request failure into one sentence plus an optional next step.
export function errorText(err: unknown): ErrorText {
  return build(err, translateShared)
}

export function useErrorText(): (err: unknown) => ErrorText {
  const { t } = useTranslation('common')
  return useCallback(
    (err: unknown) =>
      build(err, (key, defaultValue) => t(key, { defaultValue })),
    [t],
  )
}
