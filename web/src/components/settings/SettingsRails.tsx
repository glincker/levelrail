import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  PageRail,
  PageRailRow,
  PageRailSection,
} from '@/components/shell/PageRail'
import { Badge } from '@/components/ui/badge'
import { HelpLink } from '../HelpLink'
import { formatAbsolute } from '../../lib/dateFormat'
import { usePasskeys } from '../../queries/passkeys'
import { useSession } from '../../queries/security'
import { useTwoFactorStatus } from '../../queries/twoFactor'
import { useSystemStatus } from '../../queries/systemStatus'
import { updatesQueryOptions } from '../../queries/updates'
import type { BackupTarget } from '../../types/backupTarget'

const linkClass =
  'text-sm font-medium text-foreground underline underline-offset-4 hover:no-underline'

/** SecurityRail summarises sign-in protection with jumps to the cards on the page. */
export function SecurityRail() {
  const { t } = useTranslation('settings')
  const { data: twoFactor } = useTwoFactorStatus()
  const { data: passkeys } = usePasskeys()
  const { data: session } = useSession()

  return (
    <PageRail>
      <PageRailSection title={t('securityPage.rail.protection')}>
        <PageRailRow label={t('securityPage.rail.twoFactor')}>
          <Badge variant={twoFactor.enabled ? 'success' : 'warning'}>
            {twoFactor.enabled
              ? t('securityPage.rail.on')
              : t('securityPage.rail.off')}
          </Badge>
        </PageRailRow>
        <PageRailRow label={t('securityPage.rail.passkeys')}>
          {passkeys.length}
        </PageRailRow>
        <PageRailRow label={t('securityPage.rail.signedIn')}>
          {session.username}
        </PageRailRow>
        <PageRailRow label={t('securityPage.rail.until')}>
          {formatAbsolute(session.expires_at)}
        </PageRailRow>
      </PageRailSection>
      <PageRailSection title={t('securityPage.rail.jump')}>
        <ul className="space-y-1.5">
          <li>
            <a href="#sessions" className={linkClass}>
              {t('securityPage.rail.sessionsLink')}
            </a>
          </li>
          <li>
            <Link
              to="/settings/account"
              hash="two-factor"
              className={linkClass}
            >
              {t('securityPage.rail.twoFactorLink')}
            </Link>
          </li>
          <li>
            <Link to="/settings/account" hash="passkeys" className={linkClass}>
              {t('securityPage.rail.passkeysLink')}
            </Link>
          </li>
        </ul>
      </PageRailSection>
    </PageRail>
  )
}

/** GeneralRail shows where this instance is served from and the state of its core dependencies. */
export function GeneralRail() {
  const { t } = useTranslation('settings')
  const { data: status } = useSystemStatus()
  const updates = useQuery(updatesQueryOptions())
  const version = updates.data?.current_version

  return (
    <PageRail>
      <PageRailSection title={t('generalPage.rail.instance')}>
        <PageRailRow label={t('generalPage.rail.url')}>
          <span className="font-mono text-xs break-all">
            {window.location.origin}
          </span>
        </PageRailRow>
        {version ? (
          <PageRailRow label={t('generalPage.rail.version')}>
            <span className="font-mono text-xs">{version}</span>
          </PageRailRow>
        ) : null}
      </PageRailSection>
      <PageRailSection title={t('generalPage.rail.health')}>
        <PageRailRow label={t('generalPage.rail.docker')}>
          <Badge variant={status.docker_connected ? 'success' : 'destructive'}>
            {status.docker_connected
              ? t('generalPage.rail.connected')
              : t('generalPage.rail.disconnected')}
          </Badge>
        </PageRailRow>
        <PageRailRow label={t('generalPage.rail.secrets')}>
          <Badge variant={status.secrets_configured ? 'success' : 'warning'}>
            {status.secrets_configured
              ? t('generalPage.rail.configured')
              : t('generalPage.rail.notConfigured')}
          </Badge>
        </PageRailRow>
      </PageRailSection>
      <PageRailSection title={t('generalPage.rail.docs')}>
        <HelpLink
          path="/troubleshooting"
          label={t('generalPage.help')}
          variant="inline"
        />
      </PageRailSection>
    </PageRail>
  )
}

/** BackupTargetsRail counts targets and points to where backups run and are verified. */
export function BackupTargetsRail({ targets }: { targets: BackupTarget[] }) {
  const { t } = useTranslation('settings')
  const providers = Array.from(new Set(targets.map((x) => x.provider)))

  return (
    <PageRail>
      <PageRailSection title={t('backupTargetsPage.rail.targets')}>
        <PageRailRow label={t('backupTargetsPage.rail.count')}>
          {targets.length}
        </PageRailRow>
        {providers.length > 0 ? (
          <PageRailRow label={t('backupTargetsPage.rail.providers')}>
            {providers
              .map((p) => t(`backupTargetsPage.rail.provider.${p}`))
              .join(', ')}
          </PageRailRow>
        ) : null}
      </PageRailSection>
      <PageRailSection title={t('backupTargetsPage.rail.status')}>
        <p className="text-sm text-muted-foreground">
          {t('backupTargetsPage.rail.statusBody')}
        </p>
        <Link to="/backups" className={linkClass}>
          {t('backupTargetsPage.rail.statusLink')}
        </Link>
      </PageRailSection>
      <PageRailSection title={t('backupTargetsPage.rail.docs')}>
        <HelpLink
          path="/backups-and-storage"
          label={t('backupTargetsPage.help')}
          variant="inline"
        />
      </PageRailSection>
    </PageRail>
  )
}
