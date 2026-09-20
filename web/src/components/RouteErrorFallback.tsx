import { Link } from '@tanstack/react-router'
import type { ErrorComponentProps } from '@tanstack/react-router'
import {
  ArrowClockwiseIcon,
  HouseIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { routeErrorMessage } from '@/lib/apiError'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// Router-level defaultErrorComponent (see main.tsx): the fallback for any
// route without its own errorComponent. Routes that need a more specific
// back-link (apps/$name.tsx and its siblings) keep their own instead.
export function RouteErrorFallback({ error, reset }: ErrorComponentProps) {
  return (
    <div className="flex min-h-[60vh] items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="flex items-center gap-2">
            <WarningCircleIcon
              className="size-5 text-destructive"
              aria-hidden="true"
            />
            <CardTitle>Something went wrong</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {routeErrorMessage(error)}
          </p>
          <div className="flex gap-2">
            <Button type="button" variant="outline" size="sm" onClick={reset}>
              <ArrowClockwiseIcon aria-hidden="true" />
              Try again
            </Button>
            <Link to="/">
              <Button type="button" variant="ghost" size="sm">
                <HouseIcon aria-hidden="true" />
                Back to dashboard
              </Button>
            </Link>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
