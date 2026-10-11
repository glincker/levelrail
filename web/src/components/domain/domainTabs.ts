export const DOMAIN_TABS = [
  'overview',
  'redirects',
  'headers',
  'forwarding',
  'access',
  'cache',
  'ports',
  'danger',
] as const
export type DomainTab = (typeof DOMAIN_TABS)[number]
