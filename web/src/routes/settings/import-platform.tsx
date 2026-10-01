import { createFileRoute } from '@tanstack/react-router'
import { PageHeader } from '@/components/shell/PageHeader'
import { PlatformImportCard } from '../../components/PlatformImportCard'

// Needs write:sensitive server-side (the source credential travels in the
// request body), so a read-only session sees the API's 403 in the form.
export const Route = createFileRoute('/settings/import-platform')({
  component: ImportPlatformPage,
})

function ImportPlatformPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Import from another platform"
        description="Bring apps and databases over from Coolify, Dokploy or CapRover. Databases and volumes start empty, and re-running skips what was already imported."
      />
      <PlatformImportCard />
    </div>
  )
}
