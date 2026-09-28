// Display helpers for digest-pinned image references ("repo:tag@sha256:...").

export function shortDigest(digest: string): string {
  return digest.replace(/^sha256:/, '').slice(0, 12)
}

export function unpinnedImage(image: string): string {
  const at = image.lastIndexOf('@')
  return at > 0 ? image.slice(0, at) : image
}

export interface DeploySafetyValues {
  pull: boolean
  overrideReason: string
}

export const EMPTY_DEPLOY_SAFETY: DeploySafetyValues = {
  pull: false,
  overrideReason: '',
}
