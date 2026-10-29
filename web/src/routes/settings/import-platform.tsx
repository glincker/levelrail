import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeftRightGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { useTranslation } from 'react-i18next'
import { AppImportWizard } from '../../components/AppImportWizard'
import { PlatformImportCard } from '../../components/PlatformImportCard'
import { MigrationHub } from '../../components/MigrationHub'

// Needs write:sensitive server-side (the source credential travels in the
// request body), so a read-only session sees the API's 403 in the form.
export const Route = createFileRoute('/settings/import-platform')({
  component: ImportPlatformPage,
})

function ImportPlatformPage() {
  const { t } = useTranslation('migration')
  const { t: ts } = useTranslation('settings')
  return (
    <PageTemplate
      icon={<ArrowLeftRightGlassIcon size="1.4em" />}
      title={ts('importPlatformPage.title')}
      description={ts('importPlatformPage.description')}
    >
      <div className="space-y-6">
        <Link
          to="/migrate"
          className="inline-block text-sm text-primary underline-offset-4 hover:underline"
        >
          {ts('importPlatformPage.checklistLink')}
        </Link>
        <AppImportWizard />
        <PlatformImportCard />
        <div className="space-y-1">
          <h2 className="text-base font-semibold">{t('title')}</h2>
          <p className="text-sm text-muted-foreground">{t('description')}</p>
        </div>
        <MigrationHub />
      </div>
    </PageTemplate>
  )
}
