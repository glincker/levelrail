import type { BetaFeature, BetaMaturity } from '../types/resourceAdvisor'

const MATURITY_ORDER: BetaMaturity[] = ['beta', 'experimental']

export interface BetaFeatureGroup {
  maturity: BetaMaturity
  items: BetaFeature[]
}

// Beta first, then experimental; empty groups are dropped.
export function groupByMaturity(
  features: readonly BetaFeature[],
): BetaFeatureGroup[] {
  return MATURITY_ORDER.map((maturity) => ({
    maturity,
    items: features.filter((f) => f.maturity === maturity),
  })).filter((g) => g.items.length > 0)
}

export type BetaBanner =
  | { kind: 'none' }
  | { kind: 'unknown' }
  | { kind: 'locked'; feature: BetaFeature }
  | { kind: 'ready'; feature: BetaFeature }

// What the top of the page says for a ?feature= visit. 'ready' means the
// feature is on now, so a ?from= return link makes sense.
export function bannerFor(
  features: readonly BetaFeature[],
  featureId: string | undefined,
): BetaBanner {
  if (!featureId) return { kind: 'none' }
  const feature = features.find((f) => f.id === featureId)
  if (!feature) return { kind: 'unknown' }
  return feature.enabled
    ? { kind: 'ready', feature }
    : { kind: 'locked', feature }
}
