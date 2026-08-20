import { StackSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useEnvironments } from '../queries/environments'
import { CreateEnvironmentDialog } from './CreateEnvironmentDialog'
import { DeleteEnvironmentDialog } from './DeleteEnvironmentDialog'

// A project's environments (staging, production, ...), primed by
// routes/projects/$id.tsx's own loader via environmentListQueryOptions,
// the same suspense shape that route already uses for apps/databases.
// Tagging an app with one happens on the app's own Overview page
// (MoveToEnvironmentDialog), not here, mirroring how project assignment
// itself is set from an app/database's Overview, not from this page.
export function ProjectEnvironmentsPanel({
  projectId,
}: {
  projectId: string
}) {
  const { data: environments } = useEnvironments(projectId)

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3 space-y-0">
        <CardTitle className="flex items-center gap-2">
          <StackSimpleIcon className="size-4" />
          Environments
        </CardTitle>
        <CreateEnvironmentDialog projectId={projectId} />
      </CardHeader>
      <CardContent>
        {environments.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No environments yet. Create staging or production to tag apps
            with, from any app&apos;s Overview page.
          </p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {environments.map((env) => (
              <Badge
                key={env.id}
                variant="outline"
                className="gap-1.5 py-1 pr-1"
              >
                {env.name}
                <DeleteEnvironmentDialog
                  id={env.id}
                  name={env.name}
                  projectId={projectId}
                />
              </Badge>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
