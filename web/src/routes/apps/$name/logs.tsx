import { createFileRoute } from '@tanstack/react-router'
import {
  ArchiveIcon,
  TerminalIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useDeployStatus } from '../../../queries/deploys'
import { LogSearchPanel } from '../../../components/LogSearchPanel'
import { LiveLogViewer } from '../../../components/LiveLogViewer'
import { LogArchivePanel } from '../../../components/LogArchivePanel'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { parseLogsSearch, type LogsTab } from '../../../lib/observabilitySearch'

// App-scoped logs section: a live-tailing terminal plus stored search and
// the archive. Search filters and the time window live in the URL (`tab`,
// `q`, `level`, `container`, `stream`, `field`, `from`, `to`) so a link from
// a metrics spike or a copied permalink reproduces the exact view.
export const Route = createFileRoute('/apps/$name/logs')({
  validateSearch: parseLogsSearch,
  component: LogsSection,
})

function LogsSection() {
  const { name } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  // Same cache key the Overview route primes: lets an empty live tail say
  // why (the app's current reconcile condition).
  const { data: conditions } = useDeployStatus(name)
  const tab: LogsTab = search.tab ?? (search.from ? 'search' : 'live')

  return (
    <Tabs
      value={tab}
      onValueChange={(next) => {
        void navigate({
          search: (prev) => ({ ...prev, tab: next as LogsTab }),
          replace: true,
        })
      }}
      className="flex h-full flex-col gap-3"
    >
      <TabsList className="w-fit">
        <TabsTrigger value="live" className="gap-1.5">
          <TerminalIcon className="size-4" aria-hidden="true" />
          Live
        </TabsTrigger>
        <TabsTrigger value="search" className="gap-1.5">
          <MagnifyingGlassIcon className="size-4" aria-hidden="true" />
          Search
        </TabsTrigger>
        <TabsTrigger value="archive" className="gap-1.5">
          <ArchiveIcon className="size-4" aria-hidden="true" />
          Archive
        </TabsTrigger>
      </TabsList>

      <TabsContent value="live" className="flex-1">
        <LiveLogViewer appName={name} conditions={conditions} />
      </TabsContent>
      <TabsContent value="search" className="flex-1">
        <LogSearchPanel
          appName={name}
          search={search}
          onSearchChange={(next) => {
            void navigate({
              search: { ...next, tab: 'search' },
              replace: true,
            })
          }}
        />
      </TabsContent>
      <TabsContent value="archive" className="flex-1 overflow-y-auto">
        <LogArchivePanel appName={name} />
      </TabsContent>
    </Tabs>
  )
}
