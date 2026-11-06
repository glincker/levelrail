import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { useDoctorReachability } from '../../queries/doctorReachability'

// Compact status for the Domains page: one pill and a link to the full
// checklist. Renders nothing while loading, on error or when forbidden.
export function ReachabilityPill({
  fallback = null,
}: {
  fallback?: ReactNode
}) {
  const { t } = useTranslation('domains')
  const { data } = useDoctorReachability()
  if (!data) return <>{fallback}</>
  const ok = data.status === 'ok'
  return (
    <span className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
      <Badge variant={ok ? 'success' : 'warning'}>
        {ok ? t('reachability.pillOk') : t('reachability.pillWarn')}
      </Badge>
      <Link
        to="/settings/system-status"
        className="font-medium text-foreground underline underline-offset-4 hover:text-primary"
      >
        {t('reachability.openChecklist')}
      </Link>
    </span>
  )
}
