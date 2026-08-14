import { createFileRoute } from '@tanstack/react-router'
import {
  BookOpenIcon,
  LifebuoyIcon,
  GearIcon,
  SparkleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useBrand } from '../../hooks/useBrand'

// Real content for the General settings page (CLAUDE.md section 8's
// "Frontend pages, one agent per route, against a frozen API contract"
// parallel-work pattern: Account and Security are separate in-flight
// agents building out their own placeholder routes, API tokens is
// already done, this one is General).
//
// Everything here reads from the already-warm /api/v1/brand cache via
// useBrand() (primed by routes/__root.tsx's loader, see BrandProvider),
// no new query module needed: this page is read-only display of data
// that's already fetched. There is deliberately no system-status card
// here (Docker health, secrets/webhook config, disk, uptime, version):
// none of that exists as a backend endpoint yet, and CLAUDE.md 7 rules
// out inventing placeholder data for something not real.
export const Route = createFileRoute('/settings/general')({
  component: GeneralSettingsPage,
})

function GeneralSettingsPage() {
  const brand = useBrand()
  const displayName = brand.ShortName || brand.Name

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">General</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          System status and configuration.
        </p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <GearIcon className="size-4" />
            </div>
            <div>
              <CardTitle>{displayName}</CardTitle>
              <CardDescription>Platform information</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            You&apos;re running {displayName}, a self-hosted deployment
            platform: push to a git repo and get a running app back, with TLS,
            logs, metrics, and rollback handled for you. This instance and
            everything it manages runs on your own infrastructure.
          </p>
          {(brand.SupportURL || brand.DocsURL) && (
            <div className="flex flex-wrap gap-2 pt-1">
              {brand.SupportURL ? (
                <Button
                  variant="outline"
                  size="sm"
                  render={
                    <a
                      href={brand.SupportURL}
                      target="_blank"
                      rel="noreferrer"
                    />
                  }
                >
                  <LifebuoyIcon />
                  <span>Support</span>
                </Button>
              ) : null}
              {brand.DocsURL ? (
                <Button
                  variant="outline"
                  size="sm"
                  render={
                    <a href={brand.DocsURL} target="_blank" rel="noreferrer" />
                  }
                >
                  <BookOpenIcon />
                  <span>Documentation</span>
                </Button>
              ) : null}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <CardTitle>More settings, coming later</CardTitle>
            <Badge variant="muted">Planned</Badge>
          </div>
          <CardDescription>
            System status (connection health, resource usage) and notification
            channels aren&apos;t wired up on the backend yet. When they land,
            they&apos;ll show up here rather than as a separate page.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-start gap-2 text-sm text-muted-foreground">
            <SparkleIcon className="mt-0.5 size-4 shrink-0" />
            <span>
              For now this page just confirms which instance you&apos;re on and
              where to go for help.
            </span>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
