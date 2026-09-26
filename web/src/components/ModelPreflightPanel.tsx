import { useMemo } from 'react'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import {
  FIT_LABEL,
  diskTone,
  fitTone,
  formatBytes,
  parseHfRef,
  statusLabel,
  statusTone,
  tokenFingerprint,
  withQuant,
  type HfRef,
} from '../lib/modelPreflight'
import { LOCAL_NODE } from '../lib/modelDeployForm'
import { usePreflight } from '../queries/modelPreflight'
import type { ModelEngine } from '../types/models'
import type {
  PreflightQuant,
  PreflightRequest,
  PreflightResult,
} from '../types/modelPreflight'
import { InfoTip, SkeletonLine, StatusPill } from './kit'

const DEBOUNCE_MS = 500

export interface ModelPreflightPanelProps {
  engine: ModelEngine
  model: string
  node: string
  hfToken: string
  onPickModel: (model: string) => void
}

// Live Hugging Face check under the model field: existence, gated access,
// size, quantizations with a fit estimate, and free disk on the node.
export function ModelPreflightPanel({
  engine,
  model,
  node,
  hfToken,
  onPickModel,
}: ModelPreflightPanelProps) {
  const debouncedModel = useDebouncedValue(model, DEBOUNCE_MS)
  const debouncedToken = useDebouncedValue(hfToken.trim(), DEBOUNCE_MS)
  const ref = useMemo(
    () => parseHfRef(engine, debouncedModel),
    [engine, debouncedModel],
  )
  const req = useMemo<PreflightRequest | null>(() => {
    if (!ref) return null
    return {
      repo: ref.repo,
      engine,
      quant: ref.quant || undefined,
      node_id: node === LOCAL_NODE ? undefined : node,
      hf_token: debouncedToken || undefined,
    }
  }, [ref, engine, node, debouncedToken])
  const query = usePreflight(req, tokenFingerprint(debouncedToken))

  if (!ref) return null
  const settling = model !== debouncedModel || query.isFetching
  return (
    <section
      aria-label="Hugging Face check"
      aria-busy={settling}
      className="space-y-2 rounded-lg border border-border bg-muted/30 p-3 text-xs"
    >
      {query.isPending ? (
        <div className="space-y-1.5" role="status">
          <span className="sr-only">Checking Hugging Face</span>
          <SkeletonLine width="66%" />
          <SkeletonLine width="50%" />
        </div>
      ) : query.isError ? (
        <p role="alert" className="text-destructive">
          Could not run the check: {query.error.message}. You can still deploy;
          the engine downloads the weights itself.
        </p>
      ) : (
        <PreflightBody
          result={query.data}
          engine={engine}
          hfRef={ref}
          stale={settling}
          onPickModel={onPickModel}
        />
      )}
    </section>
  )
}

function PreflightBody({
  result,
  engine,
  hfRef,
  stale,
  onPickModel,
}: {
  result: PreflightResult
  engine: ModelEngine
  hfRef: HfRef
  stale: boolean
  onPickModel: (model: string) => void
}) {
  const known = result.exists
  return (
    <div className={stale ? 'space-y-2 opacity-70' : 'space-y-2'}>
      <div className="flex flex-wrap items-center gap-2">
        <StatusPill
          tone={statusTone(result.status)}
          label={statusLabel(result.status)}
          size="sm"
        />
        {result.gated ? (
          <span className="inline-flex items-center gap-1 text-muted-foreground">
            gated
            <InfoTip label="What is a gated repository">
              The publisher requires you to accept a license on Hugging Face
              before downloading. The engine then needs an access token from an
              account that accepted it.
            </InfoTip>
          </span>
        ) : null}
        {known && result.license ? (
          <span className="text-muted-foreground">
            License: {result.license}
          </span>
        ) : null}
        {known ? (
          <span className="text-muted-foreground">
            {formatBytes(result.total_bytes)} in {result.file_count} files
          </span>
        ) : null}
      </div>

      <p role="status" className="text-foreground">
        {result.message}
        {result.retry_after_seconds
          ? ` Retry in ${result.retry_after_seconds}s.`
          : ''}
      </p>
      {result.next_step ? (
        <p className="text-muted-foreground">Next: {result.next_step}</p>
      ) : null}

      {known ? (
        <>
          {result.quants.length > 0 && engine !== 'vllm' ? (
            <QuantPicker
              quants={result.quants}
              selected={hfRef.quant}
              onPick={(q) => {
                onPickModel(withQuant(hfRef, q))
              }}
            />
          ) : null}
          {result.selected ? (
            <p className="flex flex-wrap items-center gap-2 text-muted-foreground">
              <span>
                Will download {result.selected.label},{' '}
                {formatBytes(result.selected.bytes)}
              </span>
              {result.selected.fit !== 'unknown' ? (
                <StatusPill
                  tone={fitTone(result.selected.fit)}
                  label={FIT_LABEL[result.selected.fit]}
                  size="sm"
                  title="Estimate against the node's free VRAM"
                />
              ) : null}
            </p>
          ) : null}
          <p className="text-muted-foreground">{result.engine_hint}</p>
          <DiskLine result={result} />
          {result.recommendation_note ? (
            <p className="text-muted-foreground">
              {result.recommendation_note}
            </p>
          ) : null}
          {result.warnings.map((w) => (
            <p key={w} className="text-warning">
              {w}
            </p>
          ))}
          <p className="text-muted-foreground/80">{result.estimate_note}</p>
        </>
      ) : null}
    </div>
  )
}

function QuantPicker({
  quants,
  selected,
  onPick,
}: {
  quants: PreflightQuant[]
  selected: string
  onPick: (quant: string) => void
}) {
  return (
    <div className="space-y-1.5">
      <p className="flex items-center gap-1 font-medium text-foreground">
        Quantization
        <InfoTip label="What is a quantization">
          GGUF is the file format llama.cpp and Ollama load. A quant such as
          Q4_K_M stores the weights in fewer bits: smaller and faster, slightly
          less accurate. Higher numbers keep more quality and need more memory.
        </InfoTip>
      </p>
      <ul className="grid gap-1">
        {quants.map((q) => {
          const active = selected.toLowerCase() === q.name.toLowerCase()
          return (
            <li key={q.name}>
              <button
                type="button"
                aria-pressed={active}
                onClick={() => {
                  onPick(q.name)
                }}
                className={`flex w-full items-center gap-2 rounded-md border px-2 py-1 text-left transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none ${
                  active ? 'border-primary bg-muted' : 'border-border'
                }`}
              >
                <span className="font-mono text-foreground">{q.name}</span>
                <span className="text-muted-foreground">
                  {formatBytes(q.bytes)}
                </span>
                {q.recommended ? (
                  <span className="text-primary">recommended</span>
                ) : null}
                {q.fit !== 'unknown' ? (
                  <span className="ml-auto">
                    <StatusPill
                      tone={fitTone(q.fit)}
                      label={FIT_LABEL[q.fit]}
                      size="sm"
                      title="Estimate against the node's free VRAM"
                    />
                  </span>
                ) : null}
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function DiskLine({ result }: { result: PreflightResult }) {
  const { disk } = result
  return (
    <p className="flex flex-wrap items-center gap-2 text-muted-foreground">
      {disk.status !== 'unknown' ? (
        <StatusPill
          tone={diskTone(disk.status)}
          label={`Disk ${disk.status === 'insufficient' ? 'too small' : disk.status}`}
          size="sm"
        />
      ) : null}
      <span>
        {disk.message}
        {disk.free_bytes !== null && disk.required_bytes > 0
          ? ` Needs ${formatBytes(disk.required_bytes)}, ${formatBytes(disk.free_bytes)} free.`
          : ''}
      </span>
    </p>
  )
}
