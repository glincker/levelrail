import { createFileRoute } from '@tanstack/react-router'
import { PlatformImportCard } from '../../components/PlatformImportCard'

// Needs write:sensitive server-side (the source credential travels in the
// request body), so a read-only session sees the API's 403 in the form.
export const Route = createFileRoute('/settings/import-platform')({
  component: ImportPlatformPage,
})

function ImportPlatformPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">
          Import from another platform
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Bring apps and databases over from Coolify, Dokploy or CapRover.
          Databases and volumes start empty, and re-running skips what was
          already imported.
        </p>
      </div>
      <PlatformImportCard />
    </div>
  )
}
