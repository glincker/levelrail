import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GaugeIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import {
  useAppBuildLimits,
  useBuildStats,
  useResetAppBuildLimits,
  useSetAppBuildLimits,
  type AppBuildLimits,
} from '../queries/buildPerformance'

const MINUTE = 60

function minutesOf(seconds: number): string {
  return String(Math.round((seconds / MINUTE) * 100) / 100)
}

function coresOf(millis: number): string {
  return String(millis / 1000)
}

function LimitsForm({ app, limits }: { app: string; limits: AppBuildLimits }) {
  const { t } = useTranslation('buildPerformance')
  const isRoot = useIsRoot()
  const save = useSetAppBuildLimits(app)
  const reset = useResetAppBuildLimits(app)
  const stats = useBuildStats(app)
  const [minutes, setMinutes] = useState(
    minutesOf(limits.effective.timeout_seconds),
  )
  const [memory, setMemory] = useState(String(limits.effective.memory_mb))
  const [cores, setCores] = useState(coresOf(limits.effective.cpu_millis))

  const minutesNum = Number(minutes)
  const memoryNum = Number(memory)
  const coresNum = Number(cores)
  const valid =
    [minutes, memory, cores].every((v) => v.trim() !== '') &&
    minutesNum >= 0 &&
    Number.isInteger(memoryNum) &&
    memoryNum >= 0 &&
    coresNum >= 0

  const lastStatus = stats.data?.runs[0]?.status
  const eff = limits.effective
  const raise =
    lastStatus === 'memory_limit' && eff.memory_mb > 0
      ? {
          label: t('limits.raiseMemory', { value: eff.memory_mb * 2 }),
          hit: t('limits.hit.memory_limit', { value: eff.memory_mb }),
          req: { memory_mb: eff.memory_mb * 2 },
        }
      : lastStatus === 'timeout' && eff.timeout_seconds > 0
        ? {
            label: t('limits.raiseTimeout', {
              value: minutesOf(eff.timeout_seconds * 2),
            }),
            hit: t('limits.hit.timeout', {
              value: minutesOf(eff.timeout_seconds),
            }),
            req: { timeout_seconds: eff.timeout_seconds * 2 },
          }
        : null

  const badge = (set: boolean) =>
    set ? (
      <Badge variant="outline" className="ml-2">
        {t('limits.overridden')}
      </Badge>
    ) : null

  return (
    <div className="space-y-4">
      {raise ? (
        <div className="flex flex-wrap items-center gap-3 rounded-md border p-3">
          <p className="text-sm">{raise.hit}</p>
          <Button
            type="button"
            size="sm"
            disabled={!isRoot || save.isPending}
            onClick={() => {
              save.mutate(raise.req, {
                onSuccess: () => {
                  toast.add({ title: t('limits.raised'), type: 'success' })
                },
              })
            }}
          >
            {raise.label}
          </Button>
        </div>
      ) : null}
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (!valid) return
          save.mutate(
            {
              timeout_seconds: Math.round(minutesNum * MINUTE),
              memory_mb: memoryNum,
              cpu_millis: Math.round(coresNum * 1000),
            },
            {
              onSuccess: () => {
                toast.add({ title: t('limits.saved'), type: 'success' })
              },
            },
          )
        }}
      >
        <div className="grid gap-4 sm:grid-cols-3">
          <Field>
            <FieldLabel htmlFor="app-build-timeout">
              {t('limits.timeoutLabel')}
              {badge(limits.overrides.timeout_seconds !== undefined)}
            </FieldLabel>
            <Input
              id="app-build-timeout"
              type="number"
              min={0}
              step="any"
              inputMode="decimal"
              value={minutes}
              disabled={!isRoot}
              onChange={(e) => {
                setMinutes(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="app-build-memory">
              {t('limits.memoryLabel')}
              {badge(limits.overrides.memory_mb !== undefined)}
            </FieldLabel>
            <Input
              id="app-build-memory"
              type="number"
              min={0}
              step={1}
              inputMode="numeric"
              value={memory}
              disabled={!isRoot}
              onChange={(e) => {
                setMemory(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="app-build-cpu">
              {t('limits.cpuLabel')}
              {badge(limits.overrides.cpu_millis !== undefined)}
            </FieldLabel>
            <Input
              id="app-build-cpu"
              type="number"
              min={0}
              step="any"
              inputMode="decimal"
              value={cores}
              disabled={!isRoot}
              onChange={(e) => {
                setCores(e.target.value)
              }}
            />
          </Field>
        </div>
        <p className="text-sm text-muted-foreground">
          {t('limits.globalNote', {
            timeout: minutesOf(limits.global.timeout_seconds),
            memory: limits.global.memory_mb,
            cpu: coresOf(limits.global.cpu_millis),
          })}
        </p>
        {!isRoot ? (
          <p className="text-sm text-muted-foreground">
            {t('limits.rootOnly')}
          </p>
        ) : null}
        {save.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {save.error.message}
          </p>
        ) : null}
        <div className="flex gap-2">
          <Button type="submit" disabled={!isRoot || !valid || save.isPending}>
            {t('limits.save')}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={
              !isRoot ||
              reset.isPending ||
              Object.keys(limits.overrides).length === 0
            }
            onClick={() => {
              reset.mutate(undefined, {
                onSuccess: () => {
                  toast.add({ title: t('limits.resetDone'), type: 'success' })
                },
              })
            }}
          >
            {t('limits.reset')}
          </Button>
        </div>
      </form>
    </div>
  )
}

export function BuildLimitsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('buildPerformance')
  const { data, isPending, error } = useAppBuildLimits(appName)
  if (isPending) return <CardSkeleton titleWidth="w-44" rows={3} />
  return (
    <Card id="build-limits">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GaugeIcon aria-hidden="true" className="size-4" />
          {t('limits.title')}
        </CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('limits.description')}
        </p>
      </CardHeader>
      <CardContent>
        {error ? (
          <p role="alert" className="text-sm text-muted-foreground">
            {t('limits.loadError')}
          </p>
        ) : null}
        {data ? (
          <LimitsForm
            key={JSON.stringify(data.effective)}
            app={appName}
            limits={data}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
