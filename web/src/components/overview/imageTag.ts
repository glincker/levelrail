import { unpinnedImage } from '../../lib/imageDigest'

export function imageTagOf(image: string): string {
  const bare = unpinnedImage(image)
  const colon = bare.lastIndexOf(':')
  return colon > bare.lastIndexOf('/') ? bare.slice(colon + 1) : 'latest'
}
