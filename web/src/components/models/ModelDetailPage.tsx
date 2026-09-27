import { Link } from '@tanstack/react-router'
import { ArrowLeftIcon } from '@phosphor-icons/react/dist/ssr'
import { StatusPill } from '@/components/kit'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  MODEL_TABS,
  modelStatusLabel,
  modelTone,
  parseModelTab,
  type ModelTab,
} from '../../lib/modelPresentation'
import { useModel } from '../../queries/models'
import { DeleteModelDialog } from '../DeleteModelDialog'
import { LiveModelLogViewer } from '../LiveModelLogViewer'
import { ModelKeysPanel } from '../ModelKeysPanel'
import { ModelUsageCard } from '../ModelUsageCard'
import { ModelOverviewTab } from './ModelOverviewTab'

const TAB_LABEL: Record<ModelTab, string> = {
  overview: 'Overview',
  keys: 'Keys',
  usage: 'Usage',
  logs: 'Logs',
}

export function ModelDetailPage({
  name,
  tab,
  onTabChange,
}: {
  name: string
  tab: ModelTab
  onTabChange: (tab: ModelTab) => void
}) {
  const { data: model } = useModel(name)
  if (!model) return null
  const deleting = model.status.reason === 'Deleting'
  return (
    <div className="space-y-6">
      <Link
        to="/models"
        className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeftIcon aria-hidden="true" className="size-3.5" />
        AI models
      </Link>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <h1 className="text-lg font-semibold text-foreground">
            {model.name}
          </h1>
          <Badge variant="outline" className="font-mono text-[11px]">
            {model.engine}
          </Badge>
          <StatusPill
            tone={modelTone(model)}
            label={modelStatusLabel(model)}
            live={!model.status.ready && !deleting}
          />
        </div>
        <DeleteModelDialog name={model.name} disabled={deleting} />
      </div>
      <Tabs
        value={tab}
        onValueChange={(v) => {
          onTabChange(parseModelTab(v))
        }}
      >
        <TabsList variant="line">
          {MODEL_TABS.map((t) => (
            <TabsTrigger key={t} value={t}>
              {TAB_LABEL[t]}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="overview" className="pt-4">
          <ModelOverviewTab model={model} />
        </TabsContent>
        <TabsContent value="keys" className="pt-4">
          <ModelKeysPanel modelName={model.name} baseUrl={model.endpoint_url} />
        </TabsContent>
        <TabsContent value="usage" className="pt-4">
          <ModelUsageCard modelName={model.name} />
        </TabsContent>
        <TabsContent value="logs" className="pt-4">
          {tab === 'logs' ? (
            <LiveModelLogViewer modelName={model.name} />
          ) : null}
        </TabsContent>
      </Tabs>
    </div>
  )
}
