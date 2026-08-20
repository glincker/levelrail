import { createFileRoute } from '@tanstack/react-router'
import {
  TerminalIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useDeployStatus } from '../../../queries/deploys'
import { LogSearchPanel } from '../../../components/LogSearchPanel'
import { LiveLogViewer } from '../../../components/LiveLogViewer'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'

// App-scoped logs section: a live-tailing terminal (default tab, the
// "watch it happen right now" case) plus the pre-existing historical
// full-text search (LogSearchPanel, unchanged, the "why was this app
// slow at 3am last Tuesday" case), both scoped to this app and reachable
// from the same place rather than the live view orphaning search or vice
// versa. Distinct from /apps/$name/deploys/$deployId/logs, which tails
// one specific build/deploy attempt's output: this page tails the app's
// running container(s), an ongoing stream with no natural end.
//
// Client-side tab state only (no route param, no loader): switching
// tabs doesn't need to be deep-linkable the way the section routes
// themselves are, and LiveLogViewer/LogSearchPanel each own their own
// data fetching (a live SSE connection and a TanStack Query respectively),
// so there's nothing here for a loader to prime.
export const Route = createFileRoute('/apps/$name/logs')({
  component: LogsSection,
})

function LogsSection() {
  const { name } = Route.useParams()
  // Same cache key useDeployStatus already primes on the Overview
  // route, so this is free when that page was visited first: needed
  // here so an empty live tail can explain *why* (the app's own current
  // reconcile condition) instead of leaving an operator staring at a
  // blank terminal with no way to tell "still starting" from "broken".
  const { data: conditions } = useDeployStatus(name)

  return (
    <Tabs defaultValue="live" className="flex h-full flex-col gap-3">
      <TabsList className="w-fit">
        <TabsTrigger value="live" className="gap-1.5">
          <TerminalIcon className="size-4" aria-hidden="true" />
          Live
        </TabsTrigger>
        <TabsTrigger value="search" className="gap-1.5">
          <MagnifyingGlassIcon className="size-4" aria-hidden="true" />
          Search
        </TabsTrigger>
      </TabsList>

      <TabsContent value="live" className="flex-1">
        <LiveLogViewer appName={name} conditions={conditions} />
      </TabsContent>
      <TabsContent value="search" className="flex-1">
        <LogSearchPanel appName={name} />
      </TabsContent>
    </Tabs>
  )
}
