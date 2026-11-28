import type { TemplateCapability } from '../queries/serviceTemplates'

// Grant classes that need an explicit review and the root ability.
const REVIEWED_CLASSES = new Set([
  'docker_api',
  'host_network',
  'devices',
  'net_admin',
])

export function grantsToReview(
  capabilities: TemplateCapability[] | undefined,
): TemplateCapability[] {
  return (capabilities ?? []).filter((c) => REVIEWED_CLASSES.has(c.class))
}

export type InstallBlock =
  | { kind: 'arch' }
  | { kind: 'kernel'; module: string }
  | { kind: 'platform' }
  | { kind: 'root' }

/** Maps a refused install (422 preflight, 403 grant) to a block with fix text. */
export function classifyInstallBlock(
  status: number,
  message: string,
): InstallBlock | null {
  if (status === 403 && /needs the root ability/.test(message)) {
    return { kind: 'root' }
  }
  if (status !== 422) return null
  if (/allow_platform_mismatch/.test(message)) return { kind: 'arch' }
  const mod = /needs the (\S+) kernel module/.exec(message)
  if (mod || /kernel module missing/.test(message)) {
    return { kind: 'kernel', module: mod?.[1] ?? '' }
  }
  return { kind: 'platform' }
}
