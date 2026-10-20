import { useMemo } from 'react'
import { latestNewerTag, parseDockerHubImage } from '../lib/imageUpdate'
import { useDockerHubTags } from './dockerhub'

export interface ImageUpdate {
  current: string
  latest: string
}

/** useImageUpdate reports a newer tag in the same family for an image hosted on Docker Hub. Other registries and failed lookups report nothing. */
export function useImageUpdate(image: string): ImageUpdate | null {
  const ref = useMemo(() => parseDockerHubImage(image), [image])
  const { data } = useDockerHubTags(ref?.namespace ?? '', ref?.repo ?? null)
  return useMemo(() => {
    if (!ref || !data) return null
    const latest = latestNewerTag(
      ref.tag,
      data.map((t) => t.name),
    )
    return latest ? { current: ref.tag, latest } : null
  }, [ref, data])
}
