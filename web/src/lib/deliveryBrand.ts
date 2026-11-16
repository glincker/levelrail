import type { BrandIconName } from '../components/BrandIcon'

const FORGES: readonly BrandIconName[] = [
  'github',
  'gitlab',
  'bitbucket',
  'gitea',
  'forgejo',
]

// Provider names come from the wire; an unknown one simply gets no mark.
export function brandForProvider(provider: string): BrandIconName | undefined {
  const p = provider.trim().toLowerCase()
  return FORGES.find((f) => f === p)
}
