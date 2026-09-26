import { useMemo, useState } from 'react'
import { SkeletonLine, SkeletonTile } from '@/components/kit'
import { HelpLink } from '@/components/HelpLink'
import { toast } from '@/components/ui/toast'
import {
  configFromForm,
  formFromConfig,
  validateForm,
  type LbFormState,
} from '../../lib/loadBalancer'
import {
  useAppLoadBalancer,
  useAppLoadBalancerStatus,
  useClearAppLoadBalancer,
  useSetAppLoadBalancer,
  type LoadBalancerConfig,
} from '../../queries/appLoadBalancer'
import {
  isUnsupported,
  useLoadBalancerHistory,
  useRunLoadBalancerCheck,
  useSetUpstreamAdminState,
  type AdminState,
  type LiveUpstream,
} from '../../queries/loadBalancerLive'
import { ChangeSummaryBar } from './ChangeSummaryBar'
import { ExportTabs } from './ExportTabs'
import { LbConfigure } from './LbConfigure'
import { LbEmptyState } from './LbEmptyState'
import { LbHeader } from './LbHeader'
import { LbMetrics } from './LbMetrics'
import { LbNodeDrawer } from './LbNodeDrawer'
import { LbSuggestions } from './LbSuggestions'
import { LbTopology } from './LbTopology'
import { LbUpstreamTable } from './LbUpstreamTable'
import { RemoveDialog } from './RemoveDialog'
import { predictEffect, summarizeChanges } from './changes'
import { rollupPool } from './rollup'
import { computeSuggestions, type LbSuggestion } from './suggestions'
import { formFromPreset, type LbPreset } from './presets'

interface Props {
  appName: string
  replicas: number
  onSetReplicas?: () => void
}

function Loading() {
  return (
    <div
      className="space-y-4"
      aria-busy="true"
      aria-label="Loading load balancer"
    >
      <SkeletonLine width="40%" />
      <SkeletonTile className="h-48" />
      <SkeletonTile className="h-32" />
    </div>
  )
}

export function LoadBalancerPage({ appName, replicas, onSetReplicas }: Props) {
  const resource = useAppLoadBalancer(appName)
  const configured = resource.data?.configured ?? false
  const status = useAppLoadBalancerStatus(appName, configured)
  const history = useLoadBalancerHistory(appName, configured)
  const save = useSetAppLoadBalancer(appName)
  const clear = useClearAppLoadBalancer(appName)
  const check = useRunLoadBalancerCheck(appName)
  const admin = useSetUpstreamAdminState(appName)

  const [draft, setDraft] = useState<LbFormState | null>(null)
  const [presetNote, setPresetNote] = useState('')
  const [paused, setPaused] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const [configOpen, setConfigOpen] = useState(false)
  const [exportOpen, setExportOpen] = useState(false)
  const [removeOpen, setRemoveOpen] = useState(false)
  const [dismissed, setDismissed] = useState<Set<string>>(new Set())
  const [pendingSuggestion, setPendingSuggestion] = useState<string | null>(
    null,
  )

  const savedConfig = resource.data?.config
  const savedForm = useMemo(
    () => formFromConfig(savedConfig, replicas),
    [savedConfig, replicas],
  )
  const form = draft ?? savedForm
  const errors = validateForm(form)
  const chips = summarizeChanges(savedForm, form)
  const dirty = draft !== null && chips.length > 0

  const upstreams: LiveUpstream[] = useMemo(
    () =>
      (status.data?.upstreams ?? []).map((u) => ({
        ...u,
        admin_state:
          (u as LiveUpstream).admin_state ??
          history.byId.get(u.id)?.admin_state,
      })),
    [status.data, history.byId],
  )
  const rollup = rollupPool(upstreams)
  const algorithm = status.data?.algorithm ?? form.algorithm

  const suggestions = useMemo(
    () =>
      configured && !draft
        ? computeSuggestions({
            config: savedConfig ?? {},
            replicas,
            upstreams,
            history: history.data,
          }).filter((s) => !dismissed.has(s.id))
        : [],
    [
      configured,
      draft,
      savedConfig,
      replicas,
      upstreams,
      history.data,
      dismissed,
    ],
  )

  if (resource.isLoading) return <Loading />
  if (resource.isError) {
    return (
      <p role="alert" className="text-sm text-tone-danger">
        Could not load the load balancer: {resource.error.message}
      </p>
    )
  }

  function saveConfig(
    config: LoadBalancerConfig,
    title: string,
    undo?: LoadBalancerConfig,
  ) {
    save.mutate(config, {
      onSuccess: () => {
        setDraft(null)
        setPresetNote('')
        toast.add({
          title,
          description: 'The ingress reconciler applies it on its next pass.',
          type: 'success',
          actionProps: undo
            ? { children: 'Undo', onClick: () => save.mutate(undo) }
            : undefined,
        })
      },
      onError: (error) =>
        toast.add({
          title: 'Could not save the load balancer.',
          description: error.message,
          type: 'error',
        }),
    })
  }

  function applyPreset(preset: LbPreset) {
    setDraft(formFromPreset(preset, replicas))
    setPresetNote(preset.note ?? '')
    setConfigOpen(true)
  }

  function applySuggestion(s: LbSuggestion) {
    if (!s.apply) {
      onSetReplicas?.()
      return
    }
    const before = savedConfig ?? {}
    setPendingSuggestion(s.id)
    save.mutate(s.apply(before), {
      onSuccess: () =>
        toast.add({
          title: `Applied: ${s.title}`,
          type: 'success',
          actionProps: { children: 'Undo', onClick: () => save.mutate(before) },
        }),
      onError: (error) =>
        toast.add({
          title: 'Could not apply the suggestion.',
          description: error.message,
          type: 'error',
        }),
      onSettled: () => setPendingSuggestion(null),
    })
  }

  function runCheck() {
    check.mutate(undefined, {
      onSuccess: ({ results }) => {
        const ok = results.filter((r) => r.ok).length
        toast.add({
          title: `Check finished: ${ok} of ${results.length} passed`,
          type: ok === results.length ? 'success' : 'warning',
        })
      },
      onError: (error) =>
        toast.add({
          title: isUnsupported(error)
            ? 'Check now is not available yet.'
            : 'Check failed.',
          description: error.message,
          type: 'error',
        }),
    })
  }

  function setAdmin(id: string, state: AdminState) {
    admin.mutate(
      { id, admin_state: state },
      {
        onError: (error) =>
          toast.add({
            title: 'Could not update the upstream.',
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function remove() {
    clear.mutate(undefined, {
      onSuccess: () => {
        setRemoveOpen(false)
        setDraft(null)
        toast.add({
          title: 'Load balancer removed.',
          description: `${appName} routes to a single upstream again.`,
          type: 'success',
        })
      },
      onError: (error) =>
        toast.add({
          title: 'Could not remove the load balancer.',
          description: error.message,
          type: 'error',
        }),
    })
  }

  if (!configured && !draft) {
    return (
      <LbEmptyState
        replicas={replicas}
        creating={save.isPending}
        onCreate={(p) => saveConfig(p.config, 'Load balancer created.')}
        onPreset={applyPreset}
      />
    )
  }

  const bar = dirty ? (
    <ChangeSummaryBar
      chips={chips}
      effect={[predictEffect(form, chips), presetNote]
        .filter(Boolean)
        .join(' ')}
      blocker={Object.values(errors)[0]}
      saving={save.isPending}
      creating={!configured}
      onSave={() =>
        saveConfig(
          configFromForm(form),
          configured ? 'Load balancer saved.' : 'Load balancer created.',
        )
      }
      onRevert={() => {
        setDraft(null)
        setPresetNote('')
        save.reset()
      }}
    />
  ) : null

  return (
    <div className="space-y-5">
      {configured ? (
        <>
          <LbHeader
            rollup={rollup}
            algorithm={algorithm}
            checkSupported={history.supported}
            checking={check.isPending}
            onCheck={runCheck}
            onExport={() => setExportOpen(true)}
            onRemove={() => setRemoveOpen(true)}
          />
          {status.isLoading ? (
            <SkeletonTile className="h-48" />
          ) : status.isError ? (
            <p role="alert" className="text-sm text-tone-danger">
              Could not load upstream status: {status.error.message}
            </p>
          ) : (
            <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_220px]">
              <LbTopology
                upstreams={upstreams}
                algorithm={algorithm}
                paused={paused}
                onPausedChange={setPaused}
                onSelect={setSelected}
              />
              <LbMetrics
                upstreams={upstreams}
                rollup={rollup}
                history={history.byId}
              />
            </div>
          )}
          <LbSuggestions
            items={suggestions}
            pendingId={pendingSuggestion}
            onApply={applySuggestion}
            onDismiss={(id) => setDismissed((d) => new Set(d).add(id))}
          />
          {upstreams.length > 0 ? (
            <LbUpstreamTable
              upstreams={upstreams}
              algorithm={algorithm}
              history={history.byId}
              historySupported={history.supported}
              onOpen={setSelected}
              onAdmin={setAdmin}
            />
          ) : null}
        </>
      ) : (
        <header>
          <h2 className="text-lg font-semibold">New load balancer</h2>
          <p className="text-sm text-muted-foreground">
            Review the settings, then create it.
          </p>
        </header>
      )}

      <LbConfigure
        form={form}
        errors={errors}
        onChange={(patch) => setDraft({ ...form, ...patch })}
        open={configOpen || draft !== null}
        onOpenChange={setConfigOpen}
        onPreset={applyPreset}
      />
      {bar}

      {configured ? (
        <section
          aria-label="Export"
          className="space-y-3 rounded-2xl border p-4"
        >
          <button
            type="button"
            aria-expanded={exportOpen}
            onClick={() => setExportOpen(!exportOpen)}
            className="text-sm font-medium"
          >
            Export as code
          </button>
          {exportOpen ? <ExportTabs appName={appName} /> : null}
        </section>
      ) : null}
      <div className="flex justify-end">
        <HelpLink path="/load-balancing" label="Load balancing guide" />
      </div>

      <LbNodeDrawer
        upstream={upstreams.find((u) => u.id === selected)}
        history={selected ? history.byId.get(selected) : undefined}
        algorithm={algorithm}
        allUpstreams={upstreams}
        historySupported={history.supported}
        pending={admin.isPending}
        onClose={() => setSelected(null)}
        onAdmin={setAdmin}
      />
      <RemoveDialog
        appName={appName}
        open={removeOpen}
        pending={clear.isPending}
        onOpenChange={setRemoveOpen}
        onConfirm={remove}
      />
    </div>
  )
}
