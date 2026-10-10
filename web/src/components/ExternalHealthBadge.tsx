import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { ExternalHealthStatus } from '../types/externalDatabase'

const VARIANT: Record<
  ExternalHealthStatus,
  'success' | 'destructive' | 'muted'
> = {
  reachable: 'success',
  slow: 'muted',
  auth_failed: 'destructive',
  tls_error: 'destructive',
  unreachable: 'destructive',
  unknown: 'muted',
}

export function ExternalHealthBadge({
  status,
}: {
  status: ExternalHealthStatus
}) {
  const { t } = useTranslation('databases')
  return (
    <Badge variant={VARIANT[status]}>{t(`external.health.${status}`)}</Badge>
  )
}
