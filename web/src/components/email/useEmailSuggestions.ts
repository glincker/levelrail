import { useContext } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BrandContext } from '../../lib/brandContext'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'
import { sessionQueryOptions } from '../../queries/security'

export const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

const DOMAIN_RE = /^(?!\d+(\.\d+){3}$)[a-z0-9-]+(\.[a-z0-9-]+)+$/i

export function publicDomainOf(host: string | undefined): string | null {
  const bare = (host ?? '').trim().replace(/:\d+$/, '')
  return DOMAIN_RE.test(bare) ? bare.toLowerCase() : null
}

export interface EmailSuggestions {
  fromName: string | null
  fromAddress: string | null
  replyTo: string | null
  /** Domain the From address suggestion was built on, for the placeholder fallback. */
  domain: string | null
}

/** useEmailSuggestions derives one-click defaults from the instance. Nothing here is ever saved on its own. */
export function useEmailSuggestions(): EmailSuggestions {
  const brand = useContext(BrandContext)
  const https = useQuery(httpsStatusQueryOptions())
  const session = useQuery(sessionQueryOptions())
  const domain = publicDomainOf(https.data?.public_host)
  const owner = session.data?.username ?? ''
  return {
    fromName: brand?.Name || null,
    fromAddress: domain ? `noreply@${domain}` : null,
    replyTo: EMAIL_RE.test(owner) ? owner : null,
    domain,
  }
}
