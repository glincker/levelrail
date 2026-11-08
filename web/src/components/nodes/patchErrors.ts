import type { TFunction } from 'i18next'
import { ApiError } from '../../lib/apiError'

// Codes that mean the preview must be rebuilt before another apply.
const NEEDS_NEW_PREVIEW = new Set([
  'preview_expired',
  'preview_mismatch',
  'preview_stale',
])

export function patchErrorCode(error: unknown): string | undefined {
  return error instanceof ApiError ? error.code : undefined
}

export function needsNewPreview(error: unknown): boolean {
  const code = patchErrorCode(error)
  return code !== undefined && NEEDS_NEW_PREVIEW.has(code)
}

// Maps a coded API error to plain language with the next step.
export function patchErrorMessage(
  t: TFunction<'nodes'>,
  error: unknown,
): string {
  switch (patchErrorCode(error)) {
    case 'patch_unsupported':
      return t('patch.errors.patch_unsupported')
    case 'patch_manager_unsupported':
      return t('patch.errors.patch_manager_unsupported')
    case 'patch_in_progress':
      return t('patch.errors.patch_in_progress')
    case 'deploy_in_progress':
      return t('patch.errors.deploy_in_progress')
    case 'preview_expired':
      return t('patch.errors.preview_expired')
    case 'preview_mismatch':
      return t('patch.errors.preview_mismatch')
    case 'preview_stale':
      return t('patch.errors.preview_stale')
    case 'drain_required':
      return t('patch.errors.drain_required')
    case 'nothing_to_apply':
      return t('patch.errors.nothing_to_apply')
    case 'runtime_upgrade_needs_systemd':
      return t('patch.errors.runtime_upgrade_needs_systemd')
    default:
      return t('patch.errors.generic')
  }
}
