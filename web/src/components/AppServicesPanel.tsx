import { Link } from '@tanstack/react-router'
import {
  StackIcon,
  GlobeIcon,
  CaretRightIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useAppGroup } from '../queries/appGroup'
import { STATUS_DOT_COLOR } from '../lib/appStatus'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

// Read-only view of GET /api/v1/apps/{name}/group (internal/api/
// apps_group.go): name's sibling services under the same store.App, each
// linking to its own full app detail page (a service is a real app
// resource in its own right, GET /api/v1/apps/{name}). Renders exactly
// as well for a single-service group (one row) as for a real
// multi-service one, so this tab is always safe to show regardless of
// how many services an app currently has. A suspense query, primed by
// the route's own loader: a fetch failure surfaces through the route's
// errorComponent, the same AppDetailError every other section route
// shares (routes/apps/$name.tsx).
export function AppServicesPanel({ appName }: { appName: string }) {
  const { data: group } = useAppGroup(appName)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <StackIcon className="size-4" />
          Services
        </CardTitle>
        <CardDescription>
          {group.services.length > 1
            ? `${group.services.length} services deployed together under one app.`
            : 'This app currently has one service. Deploy an app.yaml with multiple services to fan more out under it.'}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-2 text-sm">
          <span
            className={`size-2 rounded-full ${STATUS_DOT_COLOR[group.status.variant]}`}
            aria-hidden="true"
          />
          <span className="text-muted-foreground">Group status:</span>
          <Badge variant={group.status.variant}>{group.status.label}</Badge>
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Service</TableHead>
              <TableHead>Image</TableHead>
              <TableHead>Port</TableHead>
              <TableHead>Domains</TableHead>
              <TableHead className="w-8" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {group.services.map((svc) => (
              <TableRow key={svc.name}>
                <TableCell className="font-medium">
                  <Link
                    to="/apps/$name/overview"
                    params={{ name: svc.name }}
                    className="hover:underline"
                  >
                    {svc.name}
                  </Link>
                  {svc.name === appName ? (
                    <Badge variant="muted" className="ml-2 px-1.5 text-[10px]">
                      this app
                    </Badge>
                  ) : null}
                </TableCell>
                <TableCell className="max-w-[16rem] truncate font-mono text-xs text-muted-foreground">
                  {svc.image}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {svc.port}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {svc.domains && svc.domains.length > 0 ? (
                    <span className="flex items-center gap-1">
                      <GlobeIcon
                        className="size-3.5 shrink-0"
                        aria-hidden="true"
                      />
                      {svc.domains[0]}
                      {svc.domains.length > 1
                        ? ` +${svc.domains.length - 1}`
                        : ''}
                    </span>
                  ) : (
                    <span className="italic text-muted-foreground/60">
                      no domain
                    </span>
                  )}
                </TableCell>
                <TableCell>
                  <Link to="/apps/$name/overview" params={{ name: svc.name }}>
                    <CaretRightIcon
                      className="size-3.5 text-muted-foreground"
                      aria-hidden="true"
                    />
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
