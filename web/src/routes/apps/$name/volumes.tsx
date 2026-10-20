import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { AppVolumesEditor } from '../../../components/AppVolumesEditor'
import { AppVolumeBackupsSection } from '../../../components/AppVolumeBackupsSection'
import { AppBindMountsSection } from '../../../components/AppBindMountsSection'

// Real deep-linkable route for an app's named Docker volumes: attach/
// detach (AppVolumesEditor, PUT /api/v1/apps/{name}/volumes) plus the
// existing backup history, manual trigger, restore, and schedule,
// mirroring routes/databases/$name/overview.tsx's own backups section
// for the database resource kind. Reads app data from the query cache
// the parent layout route's loader already primed (queries/apps.ts), no
// fetch of its own. AppBindMountsSection renders nothing when the app
// has no bind mounts, so this stays byte-identical to before that
// feature existed for every app that doesn't use it.
export const Route = createFileRoute('/apps/$name/volumes')({
  component: VolumesSection,
})

function VolumesSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <AppVolumesEditor appName={name} volumes={app.volumes} />
      <AppVolumeBackupsSection appName={name} volumes={app.volumes} />
      <AppBindMountsSection bindMounts={app.bind_mounts} />
    </div>
  )
}
