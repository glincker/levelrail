import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { domainsQueryOptions } from '../../queries/domains'
import { useStackByName } from '../../queries/stacks'
import { ManagedByStackBadge } from '../ManagedByStackBadge'

function useIacOn(): boolean {
  return useExperimentalFeatures().includes('iac')
}

// Hooks only run once a stack is named, so rows without one stay query-free.
function ManagedTag({ stack }: Readonly<{ stack: string }>) {
  if (!useIacOn()) return null
  // z-10 keeps the badge clickable above a row-wide link.
  return (
    <span className="relative z-10">
      <ManagedByStackBadge stack={stack} />
    </span>
  )
}

export function ManagedByStackTag({
  stack,
}: Readonly<{ stack: string | undefined }>) {
  return stack ? <ManagedTag stack={stack} /> : null
}

function ManagedBanner({ stack }: Readonly<{ stack: string }>) {
  const { t } = useTranslation('stacks')
  const iacOn = useIacOn()
  const { data } = useStackByName(iacOn ? stack : undefined)
  if (!iacOn) return null
  const locked = data?.drift_policy === 'lock'
  return (
    <Alert role="status" className="flex items-center gap-2 py-1.5">
      <ManagedByStackBadge stack={stack} />
      <AlertDescription>
        {t(locked ? 'managedNotice.lock' : 'managedNotice.warn', {
          name: stack,
        })}
      </AlertDescription>
    </Alert>
  )
}

// Edit warning for detail pages; says whether the managing stack is locked.
export function ManagedByStackBanner({
  stack,
}: Readonly<{ stack: string | undefined }>) {
  return stack ? <ManagedBanner stack={stack} /> : null
}

// Domains have no single-item endpoint, so read the cached domain list.
export function DomainManagedByStackBanner({
  domain,
}: Readonly<{ domain: string }>) {
  const { data } = useQuery(domainsQueryOptions())
  const stack = data?.find((d) => d.domain === domain)?.managed_by_stack
  return <ManagedByStackBanner stack={stack} />
}
