import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ServerGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { InfoTip } from '../kit/InfoTip'
import { useCanDeploy } from '../../hooks/useCanDeploy'
import { parseLabels, validateRunnerMode } from '../../lib/buildRunnerFormat'
import {
  useAppBuildRunner,
  useBuildRunners,
  useClearAppBuildRunner,
  useSaveAppBuildRunner,
  type AppBuildRunner,
  type RunnerMode,
} from '../../queries/buildRunners'

function PreferenceForm({
  appName,
  pref,
}: Readonly<{ appName: string; pref: AppBuildRunner }>) {
  const { t } = useTranslation('deploys')
  const canDeploy = useCanDeploy()
  const { data } = useBuildRunners()
  const save = useSaveAppBuildRunner(appName)
  const clear = useClearAppBuildRunner(appName)
  const [mode, setMode] = useState<RunnerMode>(pref.mode ?? 'auto')
  const [nodeId, setNodeId] = useState(pref.node_id)
  const [pinned, setPinned] = useState(pref.pinned)
  const [labels, setLabels] = useState((pref.labels ?? []).join(', '))
  const nodes = (data?.runners ?? []).filter((r) => r.kind === 'node')
  const error = save.error ?? clear.error
  const busy = save.isPending || clear.isPending
  const problem = validateRunnerMode(mode, nodeId)
  const isSet =
    (pref.mode ?? 'auto') !== 'auto' ||
    pref.node_id !== '' ||
    pref.pinned ||
    (pref.labels ?? []).length > 0

  const onSave = () => {
    save.mutate(
      {
        mode,
        node_id: mode === 'node' ? nodeId : '',
        pinned: mode === 'node' && pinned,
        labels: parseLabels(labels),
      },
      {
        onSuccess: () =>
          toast.add({ title: t('runnerPref.saved'), type: 'success' }),
      },
    )
  }
  const onClear = () => {
    clear.mutate(undefined, {
      onSuccess: () => {
        setMode('auto')
        setNodeId('')
        setPinned(false)
        setLabels('')
        toast.add({ title: t('runnerPref.cleared'), type: 'success' })
      },
    })
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="space-y-1.5">
          <div className="flex items-center gap-1.5">
            <Label htmlFor="runner-pref-mode">{t('runnerPref.mode')}</Label>
            <InfoTip label={t('runnerPref.modeTipLabel')}>
              {t('runnerPref.modeTip')}
            </InfoTip>
          </div>
          <NativeSelect
            id="runner-pref-mode"
            className="w-full"
            value={mode}
            disabled={!canDeploy || busy}
            onChange={(e) => {
              const next = e.target.value as RunnerMode
              setMode(next)
              if (next !== 'node') setPinned(false)
            }}
          >
            <option value="auto">{t('runnerPref.modeAuto')}</option>
            <option value="control_plane">
              {t('runnerPref.modeControlPlane')}
            </option>
            <option value="node">{t('runnerPref.modeNode')}</option>
          </NativeSelect>
        </div>
        {mode === 'node' ? (
          <div className="space-y-1.5">
            <Label htmlFor="runner-pref-node">{t('runnerPref.node')}</Label>
            <NativeSelect
              id="runner-pref-node"
              className="w-full"
              value={nodeId}
              disabled={!canDeploy || busy}
              aria-invalid={problem === 'nodeNeeded'}
              onChange={(e) => {
                setNodeId(e.target.value)
              }}
            >
              <option value="">{t('runnerPref.pickNode')}</option>
              {nodes.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name}
                </option>
              ))}
            </NativeSelect>
          </div>
        ) : null}
        <div className="space-y-1.5">
          <Label htmlFor="runner-pref-labels">{t('runnerPref.labels')}</Label>
          <Input
            id="runner-pref-labels"
            value={labels}
            placeholder={t('runnerPref.labelsPlaceholder')}
            disabled={!canDeploy || busy}
            onChange={(e) => {
              setLabels(e.target.value)
            }}
          />
        </div>
      </div>
      {mode === 'node' ? (
        <div className="flex items-center gap-3">
          <Switch
            id="runner-pref-pin"
            checked={pinned}
            disabled={!canDeploy || busy || nodeId === ''}
            onCheckedChange={setPinned}
            aria-describedby="runner-pref-pin-hint"
          />
          <div className="min-w-0">
            <Label htmlFor="runner-pref-pin">{t('runnerPref.pin')}</Label>
            <p
              id="runner-pref-pin-hint"
              className="text-xs text-muted-foreground"
            >
              {nodeId === ''
                ? t('runnerPref.pinNeedsNode')
                : t('runnerPref.pinHint')}
            </p>
          </div>
        </div>
      ) : null}
      {problem === 'nodeNeeded' ? (
        <p className="text-xs text-muted-foreground">
          {t('runnerPref.nodeNeeded')}
        </p>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error.message}
        </p>
      ) : null}
      {canDeploy ? null : (
        <p className="text-xs text-muted-foreground">
          {t('runnerPref.noAbility')}
        </p>
      )}
      <div className="flex flex-wrap items-center justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={!canDeploy || busy || !isSet}
          onClick={onClear}
        >
          {t('runnerPref.clear')}
        </Button>
        <Button
          size="sm"
          disabled={!canDeploy || busy || problem !== null}
          onClick={onSave}
        >
          {t('runnerPref.save')}
        </Button>
      </div>
    </div>
  )
}

// Where this app prefers to build; an empty preference follows the global strategy.
export function RunnerPreferenceCard({
  appName,
}: Readonly<{ appName: string }>) {
  const { t } = useTranslation('deploys')
  const { data, isPending, isError, error, refetch } =
    useAppBuildRunner(appName)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ServerGlassIcon className="size-5" aria-hidden="true" />
          {t('runnerPref.title')}
          <InfoTip label={t('runnerPref.prodTipLabel')}>
            {t('runnerPref.prodTip')}
          </InfoTip>
        </CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('runnerPref.description')}
        </p>
      </CardHeader>
      <CardContent>
        {isPending ? (
          <Skeleton className="h-24 w-full" />
        ) : isError ? (
          <div className="flex flex-wrap items-center gap-3">
            <p role="alert" className="text-sm text-destructive">
              {error.message}
            </p>
            <Button variant="outline" size="sm" onClick={() => void refetch()}>
              {t('runnerPref.retry')}
            </Button>
          </div>
        ) : (
          <PreferenceForm
            key={`${data.mode ?? 'auto'}|${data.node_id}|${String(data.pinned)}|${(data.labels ?? []).join(',')}`}
            appName={appName}
            pref={data}
          />
        )}
      </CardContent>
    </Card>
  )
}
