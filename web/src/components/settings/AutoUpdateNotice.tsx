import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import {
  policyQueryOptions,
  useDismissAutoNotice,
} from '../../queries/upgradeSchedule'

/** AutoUpdateNotice shows the one-time automatic update notice until it is dismissed. */
export function AutoUpdateNotice() {
  const { t } = useTranslation('updates')
  const policy = useQuery(policyQueryOptions())
  const dismiss = useDismissAutoNotice()
  const notice = policy.data?.notice
  if (!notice) return null
  return (
    <Banner
      variant="info"
      title={notice.text}
      action={
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            render={<Link to="/settings/updates" />}
          >
            {t('autoUpdate.notice.manage')}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={dismiss.isPending}
            onClick={() => dismiss.mutate()}
          >
            {t('autoUpdate.notice.dismiss')}
          </Button>
        </div>
      }
    />
  )
}
