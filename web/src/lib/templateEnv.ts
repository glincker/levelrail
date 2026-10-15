import type { ComposePreviewService } from '../types/compose'

export interface EnvGroups {
  required: string[]
  generated: string[]
  optional: string[]
}

// Names only: values are never read from the preview, they may be secrets.
export function groupEnvNames(services: ComposePreviewService[]): EnvGroups {
  const seen = new Set<string>()
  const groups: EnvGroups = { required: [], generated: [], optional: [] }
  for (const s of services) {
    for (const e of s.env ?? []) {
      if (seen.has(e.key)) continue
      seen.add(e.key)
      if (e.generated) groups.generated.push(e.key)
      else if (e.required) groups.required.push(e.key)
      else groups.optional.push(e.key)
    }
  }
  return groups
}
