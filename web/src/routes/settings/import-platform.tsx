import { createFileRoute } from '@tanstack/react-router'
import { PageHeader } from '@/components/shell/PageHeader'
import { useTranslation } from 'react-i18next'
import { PlatformImportCard } from '../../components/PlatformImportCard'
import { MigrationCutoverCard } from '../../components/MigrationCutoverCard'
import { MigrationDataCard } from '../../components/MigrationDataCard'
import { MigrationVolumesCard } from '../../components/MigrationVolumesCard'

// Needs write:sensitive server-side (the source credential travels in the
// request body), so a read-only session sees the API's 403 in the form.
export const Route = createFileRoute('/settings/import-platform')({
  component: ImportPlatformPage,
})

function ImportPlatformPage() {
  const { t } = useTranslation('migration')
  return (
    <div className="space-y-6">
      <PageHeader
        title="Import from another platform"
        description="Bring apps and databases over from Coolify, Dokploy or CapRover. Databases and volumes start empty, and re-running skips what was already imported."
      />
      <PlatformImportCard />
      <div className="space-y-1 pt-2">
        <h2 className="text-base font-semibold">{t('title')}</h2>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </div>
      <MigrationDataCard />
      <MigrationVolumesCard />
      <MigrationCutoverCard />
    </div>
  )
}
