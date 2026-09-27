import { CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from '@/components/kit'
import { gpuSummary, nodeLabel } from '../../lib/models'
import { chatCurlExample } from '../../lib/modelPresentation'
import type { ModelResource } from '../../types/models'
import { ModelEnginePanel } from './ModelEnginePanel'

function copy(value: string, label: string) {
  void navigator.clipboard.writeText(value).then(() => {
    toast.add({ title: `${label} copied.`, type: 'success' })
  })
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border p-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 truncate font-mono text-sm text-foreground">
        {value}
      </dd>
    </div>
  )
}

function CopyButton({ value, label }: { value: string; label: string }) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      aria-label={`Copy ${label}`}
      onClick={() => {
        copy(value, label)
      }}
    >
      <CopyIcon aria-hidden="true" />
      Copy
    </Button>
  )
}

export function ModelOverviewTab({ model }: { model: ModelResource }) {
  const url = model.endpoint_url
  const curl = url ? chatCurlExample(url, model.model) : ''
  return (
    <div className="space-y-6">
      {model.status.message ? (
        <p className="text-sm text-muted-foreground">{model.status.message}</p>
      ) : null}
      <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <Fact label="Engine" value={model.engine} />
        <Fact label="Model" value={model.model} />
        <Fact label="Node" value={nodeLabel(model.node_id)} />
        <Fact label="GPU" value={gpuSummary(model)} />
        <Fact
          label="Context length"
          value={
            model.context_length
              ? String(model.context_length)
              : 'engine default'
          }
        />
        <Fact label="Quantization" value={model.quantization || 'from model'} />
      </dl>
      <ModelEnginePanel modelName={model.name} engine={model.engine} />
      <section aria-label="Endpoint" className="space-y-2">
        <h3 className="text-sm font-semibold">Endpoint</h3>
        {url ? (
          <>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate rounded-md border border-border bg-muted px-3 py-2 font-mono text-xs">
                {url}
              </code>
              <CopyButton value={url} label="endpoint URL" />
            </div>
            <div className="flex items-start gap-2">
              <pre className="min-w-0 flex-1 overflow-x-auto rounded-md border border-border bg-muted p-3 font-mono text-xs">
                {curl}
              </pre>
              <CopyButton value={curl} label="curl example" />
            </div>
            <p className="text-xs text-muted-foreground">
              Set API_KEY to one of this model&apos;s keys (Keys tab).
            </p>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            The endpoint appears once the engine is reachable.
          </p>
        )}
      </section>
      <p className="text-xs text-muted-foreground">
        Deployed <RelativeTime at={model.created_at} />
      </p>
    </div>
  )
}
