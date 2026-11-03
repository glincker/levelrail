import type { NameserverProvider } from '../queries/domainCheck'

type KnownProvider = Exclude<NameserverProvider, 'other' | 'unknown'>

export const PROVIDER_LABELS: Record<KnownProvider, string> = {
  cloudflare: 'Cloudflare',
  route53: 'Route53',
  digitalocean: 'DigitalOcean',
  hetzner: 'Hetzner',
  namecheap: 'Namecheap',
  godaddy: 'GoDaddy',
  porkbun: 'Porkbun',
  google: 'Google Cloud DNS',
  azure: 'Azure DNS',
  ns1: 'NS1',
  desec: 'deSEC',
}

// Official record-creation docs only; omitted where the URL is not certain.
export const PROVIDER_DOCS: Partial<Record<KnownProvider, string>> = {
  cloudflare:
    'https://developers.cloudflare.com/dns/manage-dns-records/how-to/create-dns-records/',
  route53:
    'https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/resource-record-sets-creating.html',
  digitalocean:
    'https://docs.digitalocean.com/products/networking/dns/how-to/manage-records/',
  godaddy: 'https://www.godaddy.com/help/add-an-a-record-19238',
  google: 'https://cloud.google.com/dns/docs/records',
  azure:
    'https://learn.microsoft.com/en-us/azure/dns/dns-operations-recordsets-portal',
}

export function isKnownProvider(id: string | undefined): id is KnownProvider {
  return id !== undefined && Object.hasOwn(PROVIDER_LABELS, id)
}

export function providerLabel(id: string | undefined): string | null {
  return isKnownProvider(id) ? PROVIDER_LABELS[id] : null
}

// The host column of an A record: "@" for the zone apex, else the label part.
export function recordHost(domain: string, zone?: string): string {
  const d = domain.replace(/^\*\./, '').replace(/\.$/, '').toLowerCase()
  const z = (zone ?? '').replace(/\.$/, '').toLowerCase()
  if (!z || d === z) return '@'
  return d.endsWith(`.${z}`) ? d.slice(0, -(z.length + 1)) : d
}
