import { endpointHostname } from './brandLogos'

const DOCKER_HUB_HOSTS = new Set([
  'docker.io',
  'index.docker.io',
  'registry-1.docker.io',
  'registry.hub.docker.com',
])

export function isDockerHubHost(registryHost: string): boolean {
  return DOCKER_HUB_HOSTS.has(endpointHostname(registryHost))
}

const FALLBACK_TAG_PAGE_SIZE = 25

// Mirrors the server default of registry.dockerhub.tag_page_size.
export const DOCKER_HUB_TAG_PAGE_SIZE: number = (() => {
  const n = Number(import.meta.env.VITE_REGISTRY_TAG_PAGE_SIZE)
  return Number.isInteger(n) && n > 0 ? n : FALLBACK_TAG_PAGE_SIZE
})()
