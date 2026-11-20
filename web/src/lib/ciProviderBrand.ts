import type { BrandIconName } from '../components/BrandIcon'

const CI_BRANDS: Record<string, BrandIconName> = {
  github: 'github-actions',
  gitlab: 'gitlab',
  circleci: 'circleci',
  jenkins: 'jenkins',
  buildkite: 'buildkite',
}

// Provider ids come from the wire; an unknown one simply gets no mark.
export function brandForCIProvider(id: string): BrandIconName | undefined {
  return CI_BRANDS[id]
}
