import { lazy, Suspense, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CopyField } from '@/components/traffic/CopyField'
import { SETTINGS_SECTIONS, type SettingsSection } from './settingsSections'
import { cn } from '@/lib/utils'
import { useDatabase } from '../../../queries/databases'
import { MoveToProjectDialog } from '../../MoveToProjectDialog'
import { ResourceLabelsCard } from '../../ResourceLabelsCard'
import { DeleteDatabaseDialog } from '../../DeleteDatabaseDialog'

const NetworkPanel = lazy(() =>
  import('../../databaseNetwork/NetworkPanel').then((m) => ({
    default: m.NetworkPanel,
  })),
)
const AccessPanel = lazy(() =>
  import('../../databaseAccess/AccessPanel').then((m) => ({
    default: m.AccessPanel,
  })),
)
const UpgradesPanel = lazy(() =>
  import('../../databaseUpgrades/UpgradesPanel').then((m) => ({
    default: m.UpgradesPanel,
  })),
)

function GeneralSection({ name }: { name: string }) {
  const { t } = useTranslation('databases')
  const { data: database } = useDatabase(name)
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('home.settings.general.title')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid items-center gap-2 sm:grid-cols-[10rem_1fr]">
            <span className="text-sm text-muted-foreground">
              {t('home.settings.general.name')}
            </span>
            <CopyField
              value={database.name}
              label={t('home.settings.general.copyName')}
            />
            <span className="text-sm text-muted-foreground">
              {t('home.settings.general.engine')}
            </span>
            <span className="font-mono text-sm">
              {database.engine} {database.version}
            </span>
            <span className="text-sm text-muted-foreground">
              {t('overview.project')}
            </span>
            <span className="flex items-center gap-2 text-sm">
              {database.project_id ?? t('overview.noProject')}
              <MoveToProjectDialog
                kind="database"
                name={database.name}
                currentProjectId={database.project_id}
              />
            </span>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('home.settings.general.immutable')}
          </p>
        </CardContent>
      </Card>
      <ResourceLabelsCard path="databases" name={name} />
    </div>
  )
}

function DangerSection({ name }: { name: string }) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('home.settings.danger.title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {t('home.settings.danger.body')}
        </p>
        <Button variant="destructive" onClick={() => setOpen(true)}>
          {t('detail.delete')}
        </Button>
        <DeleteDatabaseDialog name={name} open={open} onOpenChange={setOpen} />
      </CardContent>
    </Card>
  )
}

// Regroups the former Network, Access and Upgrades tabs without changing
// their behavior; the section lives in the URL so old links redirect here.
export function SettingsShell({
  name,
  section,
}: {
  name: string
  section: SettingsSection
}) {
  const { t } = useTranslation('databases')
  const { data: database } = useDatabase(name)
  const sections = SETTINGS_SECTIONS.filter(
    (s) => !(database.external && s === 'backups'),
  )
  return (
    <div className="grid gap-4 md:grid-cols-[11rem_1fr]">
      <nav
        aria-label={t('home.nav.settings')}
        className="flex gap-1 md:flex-col"
      >
        {sections.map((s) => (
          <Link
            key={s}
            to="/databases/$name/settings"
            params={{ name }}
            search={{ section: s }}
            className={cn(
              'rounded-md px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
              section === s && 'bg-muted font-medium text-foreground',
            )}
            aria-current={section === s ? 'page' : undefined}
          >
            {t(`home.settings.sections.${s}`)}
          </Link>
        ))}
      </nav>
      <div className="min-w-0">
        <Suspense fallback={null}>
          {section === 'general' ? <GeneralSection name={name} /> : null}
          {section === 'networking' ? (
            <NetworkPanel databaseName={name} />
          ) : null}
          {section === 'credentials' ? (
            <AccessPanel databaseName={name} />
          ) : null}
          {section === 'backups' ? <UpgradesPanel databaseName={name} /> : null}
          {section === 'danger' ? <DangerSection name={name} /> : null}
        </Suspense>
      </div>
    </div>
  )
}
