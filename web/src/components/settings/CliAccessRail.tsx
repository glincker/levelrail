import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  PageRail,
  PageRailRow,
  PageRailSection,
} from '@/components/shell/PageRail'
import { Badge } from '@/components/ui/badge'
import { HelpLink } from '../HelpLink'

/** CliAccessRail holds the CLI page's side notes: pending logins, the API URL, the CI token hint and docs. */
export function CliAccessRail({
  pending,
  apiUrl,
}: {
  pending: number
  apiUrl: string
}) {
  const { t } = useTranslation('settings')
  return (
    <PageRail>
      <PageRailSection title={t('cliAccessPage.rail.status')}>
        <PageRailRow label={t('cliAccessPage.rail.pending')}>
          <Badge variant={pending > 0 ? 'warning' : 'muted'}>{pending}</Badge>
        </PageRailRow>
        <PageRailRow label={t('cliAccessPage.rail.apiUrl')}>
          <span className="font-mono text-xs break-all">{apiUrl}</span>
        </PageRailRow>
      </PageRailSection>
      <PageRailSection title={t('cliAccessPage.rail.ci')}>
        <p className="text-sm text-muted-foreground">
          {t('cliAccessPage.ciHint')}
        </p>
        <Link
          to="/settings/tokens"
          className="text-sm font-medium text-foreground underline underline-offset-4 hover:no-underline"
        >
          {t('cliAccessPage.ciLink')}
        </Link>
      </PageRailSection>
      <PageRailSection title={t('cliAccessPage.rail.docs')}>
        <HelpLink
          path="/cli-reference"
          label={t('cliAccessPage.help')}
          variant="inline"
        />
      </PageRailSection>
    </PageRail>
  )
}
