import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError } from './apiError'
import { translateShared } from './activeLocale'

export interface ErrorText {
  message: string
  action?: string
}

type Translate = (key: string, defaultValue: string) => string

// Server codes and statuses that map to a fixed, actionable sentence.
const CODE_KIND: Record<string, string> = {
  docker_unavailable: 'docker',
  unauthorized: 'unauthorized',
  forbidden: 'forbidden',
  conflict: 'conflict',
  validation: 'validation',
  invalid: 'validation',
  rate_limited: 'rateLimited',
  too_many_requests: 'rateLimited',
  disk_full: 'diskFull',
  insufficient_storage: 'diskFull',
}

const STATUS_KIND: Record<number, string> = {
  401: 'unauthorized',
  403: 'forbidden',
  409: 'conflict',
  422: 'validation',
  429: 'rateLimited',
  507: 'diskFull',
  502: 'unreachable',
  503: 'unreachable',
  504: 'unreachable',
}

const GENERIC_MESSAGES = new Set(['internal error', ''])

function kindOf(err: unknown): string | undefined {
  if (err instanceof ApiError) {
    return (err.code && CODE_KIND[err.code]) || STATUS_KIND[err.status]
  }
  return err instanceof TypeError ? 'network' : undefined
}

function build(err: unknown, tr: Translate): ErrorText {
  const kind = kindOf(err)
  const raw = err instanceof Error ? err.message.trim() : ''
  if (!kind) {
    const generic = GENERIC_MESSAGES.has(raw.toLowerCase())
    return {
      message: generic
        ? tr('apiError.generic', 'Something went wrong on the server.')
        : raw,
      action: generic
        ? tr(
            'apiError.friendly.generic.action',
            'Try again, and quote the request ID if it keeps failing.',
          )
        : undefined,
    }
  }
  const base = `apiError.friendly.${kind}`
  // Validation and conflict messages from the server name the exact field, so they win.
  const keepServer =
    (kind === 'validation' || kind === 'conflict') &&
    raw !== '' &&
    !GENERIC_MESSAGES.has(raw.toLowerCase())
  return {
    message: keepServer ? raw : tr(`${base}.message`, raw),
    action: tr(`${base}.action`, '') || undefined,
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
