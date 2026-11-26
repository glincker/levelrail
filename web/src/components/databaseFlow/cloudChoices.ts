import type { NodeProviderResource } from '../../types/nodeProvision'

// The providers worth offering, in display order. Others the API returns are
// appended, missing ones are marked coming soon.
const WANTED = ['aws', 'azure', 'gcp', 'oracle', 'hetzner', 'digitalocean']

export interface CloudChoice {
  id: string
  wired: boolean
  connected: boolean
}

// Pure so the CTA states can be tested without the network.
export function cloudChoices(
  providers: readonly NodeProviderResource[],
): CloudChoice[] {
  const byId = new Map(providers.map((p) => [p.provider, p]))
  const ids = [
    ...WANTED,
    ...providers.map((p) => p.provider).filter((p) => !WANTED.includes(p)),
  ]
  return ids.map((id) => {
    const p = byId.get(id)
    return {
      id,
      wired: p !== undefined,
      connected: p?.connected === true || p?.status === 'connected',
    }
  })
}
