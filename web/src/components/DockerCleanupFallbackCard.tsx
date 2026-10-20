import { StackIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { CleanUpDockerDialog } from './CleanUpDockerDialog'

export function DockerCleanupFallbackCard() {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <StackIcon className="size-4" />
            </div>
            <div>
              <CardTitle>Docker storage</CardTitle>
              <CardDescription>
                Usage details are unavailable, but cleanup still works.
              </CardDescription>
            </div>
          </div>
          <CleanUpDockerDialog />
        </div>
      </CardHeader>
    </Card>
  )
}
