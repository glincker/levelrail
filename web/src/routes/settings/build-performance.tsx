import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ClockGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  useBuildLimits,
  useResetBuildLimits,
  useSetBuildLimits,
  type BuildLimits,
} from '../../queries/buildPerformance'

export const Route = createFileRoute('/settings/build-performance')({
  component: BuildPerformancePage,
})

function toMinutes(seconds: number): string {
  return String(Math.round((seconds / 60) * 100) / 100)
}

function LimitsForm({ limits }: { limits: BuildLimits }) {
  const { t } = useTranslation('buildPerformance')
  const isRoot = useIsRoot()
  const save = useSetBuildLimits()
  const reset = useResetBuildLimits()
  const [minutes, setMinutes] = useState(
    toMinutes(limits.effective.timeout_seconds),
  )
  const [concurrent, setConcurrent] = useState(
    String(limits.effective.max_concurrent),
  )
  const [memory, setMemory] = useState(String(limits.effective.memory_mb))
  const [cores, setCores] = useState(String(limits.effective.cpu_millis / 1000))

  const minutesNum = Number(minutes)
  const concurrentNum = Number(concurrent)
  const valid =
    minutes.trim() !== '' &&
    concurrent.trim() !== '' &&
    Number.isFinite(minutesNum) &&
    minutesNum >= 0 &&
    Number.isInteger(concurrentNum) &&
    concurrentNum >= 0 &&
    concurrentNum <= 64 &&
    memory.trim() !== '' &&
    Number.isInteger(Number(memory)) &&
    Number(memory) >= 0 &&
    cores.trim() !== '' &&
    Number(cores) >= 0

  return (
    <Card>
      <CardContent className="space-y-5">
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!valid) return
            save.mutate(
              {
                timeout_seconds: Math.round(minutesNum * 60),
                max_concurrent: concurrentNum,
                memory_mb: Number(memory),
                cpu_millis: Math.round(Number(cores) * 1000),
              },
              {
                onSuccess: () => {
                  toast.add({ title: t('settings.saved'), type: 'success' })
                },
              },
            )
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="build-timeout">
                {t('settings.timeoutLabel')}
                {limits.overrides.timeout_seconds !== undefined ? (
                  <Badge variant="outline" className="ml-2">
                    {t('settings.overridden')}
                  </Badge>
                ) : null}
              </FieldLabel>
              <Input
                id="build-timeout"
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
              <FieldDescription>{t('settings.timeoutHelp')}</FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="build-concurrent">
                {t('settings.concurrentLabel')}
                {limits.overrides.max_concurrent !== undefined ? (
                  <Badge variant="outline" className="ml-2">
                    {t('settings.overridden')}
                  </Badge>
                ) : null}
              </FieldLabel>
              <Input
                id="build-concurrent"
                type="number"
                min={0}
                max={64}
                step={1}
                inputMode="numeric"
                value={concurrent}
                disabled={!isRoot}
                onChange={(e) => {
                  setConcurrent(e.target.value)
                }}
              />
              <FieldDescription>
                {t('settings.concurrentHelp')}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="build-memory">
                {t('settings.memoryLabel')}
                {limits.overrides.memory_mb !== undefined ? (
                  <Badge variant="outline" className="ml-2">
                    {t('settings.overridden')}
                  </Badge>
                ) : null}
              </FieldLabel>
              <Input
                id="build-memory"
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
              <FieldDescription>{t('settings.memoryHelp')}</FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="build-cpu">
                {t('settings.cpuLabel')}
                {limits.overrides.cpu_millis !== undefined ? (
                  <Badge variant="outline" className="ml-2">
                    {t('settings.overridden')}
                  </Badge>
                ) : null}
              </FieldLabel>
              <Input
                id="build-cpu"
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
              <FieldDescription>{t('settings.cpuHelp')}</FieldDescription>
            </Field>
          </div>
          <p className="text-sm text-muted-foreground">
            {t('settings.defaultsNote', {
              timeout: toMinutes(limits.default.timeout_seconds),
              concurrent: limits.default.max_concurrent,
              memory: limits.default.memory_mb,
              cpu: limits.default.cpu_millis / 1000,
            })}
          </p>
          {!isRoot ? (
            <p className="text-sm text-muted-foreground">
              {t('settings.rootOnly')}
            </p>
          ) : null}
          {save.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {save.error.message}
            </p>
          ) : null}
          {reset.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {reset.error.message}
            </p>
          ) : null}
          <div className="flex gap-2">
            <Button
              type="submit"
              disabled={!isRoot || !valid || save.isPending}
            >
              {t('settings.save')}
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
                  onSuccess: (next) => {
                    setMinutes(toMinutes(next.effective.timeout_seconds))
                    setConcurrent(String(next.effective.max_concurrent))
                    setMemory(String(next.effective.memory_mb))
                    setCores(String(next.effective.cpu_millis / 1000))
                    toast.add({
                      title: t('settings.resetDone'),
                      type: 'success',
                    })
                  },
                })
              }}
            >
              {t('settings.reset')}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function BuildPerformancePage() {
  const { t } = useTranslation('buildPerformance')
  const { data, isPending, error } = useBuildLimits()
  return (
    <PageTemplate
      icon={<ClockGlassIcon size="1.4em" />}
      title={t('settings.title')}
      description={t('settings.description')}
    >
      {isPending ? <Skeleton className="h-48 w-full" /> : null}
      {error ? (
        <Banner variant="critical" title={t('settings.loadError')} />
      ) : null}
      {data ? (
        <LimitsForm key={JSON.stringify(data.effective)} limits={data} />
      ) : null}
    </PageTemplate>
  )
}
