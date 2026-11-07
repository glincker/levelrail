export type NsProvider =
  'cloudflare' | 'route53' | 'godaddy' | 'namecheap' | 'google' | 'generic'

// Matched on the authoritative nameserver hostnames, which every provider
// brands in a stable way.
const PATTERNS: [Exclude<NsProvider, 'generic'>, RegExp][] = [
  ['cloudflare', /\.ns\.cloudflare\.com\.?$/i],
  ['route53', /awsdns-\d+\.(com|net|org|co\.uk)\.?$/i],
  ['godaddy', /\.domaincontrol\.com\.?$/i],
  ['namecheap', /\.registrar-servers\.com\.?$/i],
  ['google', /(googledomains\.com|ns-cloud-[a-e]\d\.googledomains\.com)\.?$/i],
]

export const PROVIDER_DNS_URL: Record<
  Exclude<NsProvider, 'generic'>,
  string
> = {
  cloudflare: 'https://dash.cloudflare.com/',
  route53: 'https://console.aws.amazon.com/route53/v2/hostedzones',
  godaddy: 'https://dcc.godaddy.com/manage/dns',
  namecheap: 'https://ap.www.namecheap.com/domains/list/',
  google: 'https://console.cloud.google.com/net-services/dns/zones',
}

export function detectNsProvider(nameservers: readonly string[]): NsProvider {
  for (const ns of nameservers) {
    const hit = PATTERNS.find(([, re]) => re.test(ns))
    if (hit) return hit[0]
  }
  return 'generic'
}
