export type ImagePin = 'digest' | 'tag' | 'floating'

/** imagePin classifies how firmly an image reference is pinned. */
export function imagePin(image: string): ImagePin {
  if (image.includes('@sha256:')) return 'digest'
  const name = image.slice(image.lastIndexOf('/') + 1)
  const tag = name.includes(':') ? name.slice(name.indexOf(':') + 1) : ''
  if (tag === '' || tag === 'latest') return 'floating'
  return 'tag'
}

export interface TrustSummary {
  digest: number
  tag: number
  floating: number
  total: number
}

/** summarizeImagePins counts pin kinds over the unique images of a template. */
export function summarizeImagePins(images: string[]): TrustSummary {
  const unique = Array.from(new Set(images.filter((i) => i !== '')))
  const out: TrustSummary = {
    digest: 0,
    tag: 0,
    floating: 0,
    total: unique.length,
  }
  for (const image of unique) out[imagePin(image)] += 1
  return out
}

export type EnvKind = 'generated' | 'required' | 'optional'

export interface EnvLike {
  generated?: boolean
  required?: boolean
}

export function envKind(e: EnvLike): EnvKind {
  if (e.generated) return 'generated'
  if (e.required) return 'required'
  return 'optional'
}
