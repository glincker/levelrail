export interface CloneDomainRewrite {
  copyDomains: boolean
  mode: 'prefix' | 'replace'
  prefix: string
  find: string
  replace: string
}

export const EMPTY_DOMAIN_REWRITE: CloneDomainRewrite = {
  copyDomains: false,
  mode: 'prefix',
  prefix: '',
  find: '',
  replace: '',
}

// domainRewritePayload is the request fields for a clone, or null when the
// operator is not copying domains or the rewrite is still incomplete.
export function domainRewritePayload(
  v: CloneDomainRewrite,
): { prefix?: string; find?: string; replace?: string } | null {
  if (!v.copyDomains) return null
  if (v.mode === 'prefix') {
    return v.prefix.trim() ? { prefix: v.prefix.trim() } : null
  }
  return v.find.trim() && v.replace.trim()
    ? { find: v.find.trim(), replace: v.replace.trim() }
    : null
}
