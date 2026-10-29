import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArchiveGlassIcon,
  BugGlassIcon,
  SearchGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { useDeployStatus } from '../../../queries/deploys'
import { LogSearchPanel } from '../../../components/LogSearchPanel'
import { LiveLogViewer } from '../../../components/LiveLogViewer'
import { LogArchivePanel } from '../../../components/LogArchivePanel'
import { LogSignaturesPanel } from '../../../components/LogSignaturesPanel'
import {
  CreateLogAlertDialog,
  type LogAlertDefaults,
} from '../../../components/CreateLogAlertDialog'
import { AppLogBudgetField } from '../../../components/logBudgets/AppLogBudgetField'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { parseLogsSearch, type LogsTab } from '../../../lib/observabilitySearch'
import { SavedLogQueriesMenu } from '../../../components/SavedLogQueriesMenu'
import { rangeSeconds, signatureToRegex } from '../../../lib/logExplorer'
import type { LogQuerySpec } from '../../../types/logExplorer'

// App-scoped logs section: a live-tailing terminal plus stored search, error
// signatures and the archive. Search filters and the time window live in the
// URL (`tab`, `q`, `level`, `container`, `stream`, `field`, `signature`,
// `from`, `to`) so a link from a metrics spike or a copied permalink
// reproduces the exact view.
export const Route = createFileRoute('/apps/$name/logs')({
  validateSearch: parseLogsSearch,
  component: LogsSection,
})

function LogsSection() {
  const { name } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const { t } = useTranslation('observability')
  // Same cache key the Overview route primes: lets an empty live tail say
  // why (the app's current reconcile condition).
  const { data: conditions } = useDeployStatus(name)
  const tab: LogsTab = search.tab ?? (search.from ? 'search' : 'live')
  const [alertDefaults, setAlertDefaults] = useState<LogAlertDefaults | null>(
    null,
  )

  const openSavedQuery = (spec: LogQuerySpec) => {
    const secs = rangeSeconds(spec.range)
    const to = spec.to ?? (secs > 0 ? new Date().toISOString() : undefined)
    const from =
      spec.from ??
      (secs > 0 ? new Date(Date.now() - secs * 1000).toISOString() : undefined)
    void navigate({
      search: {
        tab: 'search',
        q: spec.q,
        level: spec.level,
        stream: spec.stream,
        container: spec.container,
        field: spec.fields?.length ? spec.fields : undefined,
        from: from && to ? from : undefined,
        to: from && to ? to : undefined,
      },
      replace: true,
    })
  }

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
      <div className="flex flex-wrap items-center justify-between gap-2">
        <TabsList className="w-fit">
          <TabsTrigger value="live" className="gap-1.5">
            <TerminalGlassIcon className="size-5" aria-hidden="true" />
            Live
          </TabsTrigger>
          <TabsTrigger value="search" className="gap-1.5">
            <SearchGlassIcon className="size-5" aria-hidden="true" />
            Search
          </TabsTrigger>
          <TabsTrigger value="errors" className="gap-1.5">
            <BugGlassIcon className="size-5" aria-hidden="true" />
            {t('explorer.tabErrors')}
          </TabsTrigger>
          <TabsTrigger value="archive" className="gap-1.5">
            <ArchiveGlassIcon className="size-5" aria-hidden="true" />
            Archive
          </TabsTrigger>
        </TabsList>
        {tab !== 'search' ? (
          <SavedLogQueriesMenu
            appName={name}
            current={{}}
            currentSpec={{}}
            onApply={openSavedQuery}
          />
        ) : null}
      </div>
      <AppLogBudgetField app={name} />

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
      <TabsContent value="errors" className="flex-1">
        <LogSignaturesPanel
          appName={name}
          onViewLines={(sig, range) => {
            void navigate({
              search: {
                tab: 'search',
                signature: sig.id,
                level: sig.level || undefined,
                from: range.from.toISOString(),
                to: range.to.toISOString(),
              },
              replace: true,
            })
          }}
          onAlert={(sig) => {
            setAlertDefaults({
              name: t('explorer.alertDefaultName', { app: name }),
              regex: signatureToRegex(sig.template),
              level: sig.level || 'error',
            })
          }}
        />
      </TabsContent>
      <TabsContent value="archive" className="flex-1 overflow-y-auto">
        <LogArchivePanel appName={name} />
      </TabsContent>
      {alertDefaults ? (
        <CreateLogAlertDialog
          appName={name}
          open
          onOpenChange={(open) => {
            if (!open) {
              setAlertDefaults(null)
            }
          }}
          defaults={alertDefaults}
        />
      ) : null}
    </Tabs>
  )
}
