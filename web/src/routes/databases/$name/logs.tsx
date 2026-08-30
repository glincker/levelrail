import { createFileRoute } from '@tanstack/react-router'
import {
  TerminalIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useDatabaseStatus } from '../../../queries/databases'
import { DatabaseLogSearchPanel } from '../../../components/DatabaseLogSearchPanel'
import { LiveDatabaseLogViewer } from '../../../components/LiveDatabaseLogViewer'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'

// Database-scoped logs section, the database-kind counterpart to
// routes/apps/$name/logs.tsx: a live-tailing terminal (default tab) plus
// historical full-text search, both scoped to this database's own
// managed container. Client-side tab state only, same reasoning as the
// app-scoped route this mirrors: neither tab needs to be deep-linkable,
// and each of LiveDatabaseLogViewer/DatabaseLogSearchPanel owns its own
// data fetching.
export const Route = createFileRoute('/databases/$name/logs')({
  component: LogsSection,
})

function LogsSection() {
  const { name } = Route.useParams()
  // Same cache key the parent layout route's loader already primed
  // (routes/databases/$name.tsx), so this is free: needed here so an
  // empty live tail can explain *why* (this database's own current
  // reconcile condition) instead of leaving a blank terminal with no
  // way to tell "still starting" from "broken".
  const { data: conditions } = useDatabaseStatus(name)

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
        <LiveDatabaseLogViewer databaseName={name} conditions={conditions} />
      </TabsContent>
      <TabsContent value="search" className="flex-1">
        <DatabaseLogSearchPanel databaseName={name} />
      </TabsContent>
    </Tabs>
  )
}
