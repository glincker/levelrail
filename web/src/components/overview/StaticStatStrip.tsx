import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { InfoTip, type StatusState } from '@/components/kit'
import { StatTile } from '../apps/StatTile'
import { formatSize } from '../../lib/format'
import { useAppVisitors, type VisitorsWindow } from '../../queries/appVisitors'
import { usePagesSiteDetail } from '../../queries/pagesSites'
import type { DeployAttempt } from '../../types/deployAttempt'
import { ERROR_RATE_DANGER } from './config'
import { DEPLOY_STATE, errorShare } from './staticStripModel'

const DASH = '-'

/** One strip of the few numbers a static site has: traffic, deploy, size, domains. */
export function StaticStatStrip({
  appName,
  range,
  latest,
  domainCount,
}: {
  appName: string
  range: VisitorsWindow
  latest?: DeployAttempt
  domainCount: number
}) {
  const { t } = useTranslation('appOverview')
  const { t: tq } = useTranslation('appPages')
  const visitors = useAppVisitors(appName, range).data
  const site = usePagesSiteDetail(appName).data
  const share = visitors ? errorShare(visitors.requests, visitors.status) : null
  let errorState: StatusState | undefined
  if (share !== null) {
    errorState = share >= ERROR_RATE_DANGER ? 'failed' : 'healthy'
  }

  return (
    <section aria-label={t('staticStrip.label')}>
      <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6">
        <li className="min-w-0">
          <StatTile
            label={t('staticStrip.visitors', { range })}
            value={visitors ? visitors.visitors.toLocaleString() : DASH}
            hint={<InfoTip>{t('staticStrip.visitorsInfo')}</InfoTip>}
          />
        </li>
        <li className="min-w-0">
          <StatTile
            label={t('staticStrip.requests', { range })}
            value={visitors ? visitors.requests.toLocaleString() : DASH}
          />
        </li>
        <li className="min-w-0">
          <StatTile
            label={t('staticStrip.errors')}
            state={errorState}
            value={share === null ? DASH : `${(share * 100).toFixed(1)}%`}
            hint={<InfoTip>{t('staticStrip.errorsInfo')}</InfoTip>}
          />
        </li>
        <li className="min-w-0">
          <Link
            to="/apps/$name/deploys"
            params={{ name: appName }}
            className="block rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
          >
            <StatTile
              label={t('staticStrip.deploy')}
              state={latest ? DEPLOY_STATE[latest.status] : undefined}
              value={latest ? tq(`quick.deployStatus.${latest.status}`) : DASH}
            />
          </Link>
        </li>
        <li className="min-w-0">
          <StatTile
            label={t('staticStrip.size')}
            value={site ? formatSize(site.size_bytes) : DASH}
            hint={
              site && site.file_count > 0
                ? t('staticStrip.files', { count: site.file_count })
                : undefined
            }
          />
        </li>
        <li className="min-w-0">
          <StatTile
            label={t('staticStrip.domains')}
            value={
              domainCount > 0 ? (
                domainCount
              ) : (
                <Link
                  to="/apps/$name/domains"
                  params={{ name: appName }}
                  className="text-base text-primary underline underline-offset-2"
                >
                  {t('staticStrip.addDomain')}
                </Link>
              )
            }
          />
        </li>
      </ul>
    </section>
  )
}
