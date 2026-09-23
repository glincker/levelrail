import { SparkleIcon, SpinnerIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import type { DetectFrameworkResult } from '../queries/buildDetect'

// DetectionStatus is the inline "Detected: Next.js" (or "nothing
// detected") indicator GitBuildSourceFields shows next to the build-pack
// label: never blocks or hides the tabs below it, since railpack is
// already the default tab either way (see BUILD_TYPES' own default in
// CreateAppFromGitFields).
export function DetectionStatus({
  pending,
  result,
}: {
  pending: boolean
  result: DetectFrameworkResult | undefined
}) {
  if (pending) {
    return (
      <span className="flex items-center gap-1 text-xs text-muted-foreground">
        <SpinnerIcon className="size-3.5 animate-spin" aria-hidden="true" />
        Detecting...
      </span>
    )
  }
  if (result?.detected && result.frameworkName) {
    return (
      <Badge variant="muted" className="gap-1">
        <SparkleIcon className="size-3" aria-hidden="true" />
        Detected: {result.frameworkName}
      </Badge>
    )
  }
  return null
}
