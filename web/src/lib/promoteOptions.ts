import type { PromotePreviewResource } from '../types/promote'

export interface PromoteOptions {
  includeEnv: boolean
  force: boolean
  overrideFreeze: boolean
  overrideReason: string
}

export const EMPTY_PROMOTE_OPTIONS: PromoteOptions = {
  includeEnv: false,
  force: false,
  overrideFreeze: false,
  overrideReason: '',
}

// Whether the current option choices satisfy the preview's blockers and freeze.
export function promoteOptionsReady(
  preview: PromotePreviewResource,
  o: PromoteOptions,
): boolean {
  if ((preview.blockers?.length ?? 0) > 0 && !o.force) return false
  if (preview.frozen && (!o.overrideFreeze || o.overrideReason.trim() === '')) {
    return false
  }
  return true
}
