import { GaugeGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { RelativeTime } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBenchmarkModel } from '../../queries/modelsCpu'
import { formatGiB, speedRange } from '../../lib/modelsCpu'
import type { CpuFit, ModelBenchmark } from '../../types/modelsCpu'

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border p-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 font-mono text-sm text-foreground">{value}</dd>
    </div>
  )
}

// The measured speed sits next to the estimate from the preflight. The
// benchmark only runs when the operator asks, on a model that is loaded.
export function ModelBenchmarkCard({
  modelName,
  ready,
  benchmark,
  estimate,
}: {
  modelName: string
  ready: boolean
  benchmark?: ModelBenchmark
  estimate?: CpuFit
}) {
  const { t } = useTranslation('models')
  const run = useBenchmarkModel()
  const result = benchmark
  const range = estimate
    ? speedRange(estimate.tokens_per_sec_low, estimate.tokens_per_sec_high)
    : ''

  return (
    <section aria-label={t('cpu.benchmark.title')} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">{t('cpu.benchmark.title')}</h3>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!ready || run.isPending}
          onClick={() => {
            run.mutate(modelName, {
              onSuccess: () => {
                toast.add({
                  title: t('cpu.benchmark.done'),
                  type: 'success',
                })
              },
              onError: (error) => {
                toast.add({
                  title: t('cpu.benchmark.failed'),
                  description: error.message,
                  type: 'error',
                })
              },
            })
          }}
        >
          <GaugeGlassIcon size="1.2em" aria-hidden="true" />
          {run.isPending ? t('cpu.benchmark.running') : t('cpu.benchmark.run')}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        {ready ? t('cpu.benchmark.hint') : t('cpu.benchmark.notReady')}
      </p>
      {result ? (
        <>
          <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Stat
              label={t('cpu.benchmark.measured')}
              value={t('cpu.benchmark.tps', {
                value: result.gen_tokens_per_sec.toFixed(1),
              })}
            />
            <Stat
              label={t('cpu.benchmark.estimated')}
              value={
                range
                  ? t('cpu.benchmark.tps', { value: range })
                  : t('cpu.benchmark.noEstimate')
              }
            />
            <Stat
              label={t('cpu.benchmark.prompt')}
              value={t('cpu.benchmark.tps', {
                value: result.prompt_tokens_per_sec.toFixed(1),
              })}
            />
            <Stat
              label={t('cpu.benchmark.ttft')}
              value={t('cpu.benchmark.ms', { value: result.ttft_ms })}
            />
          </dl>
          <p className="text-xs text-muted-foreground">
            {result.peak_memory_bytes > 0
              ? t('cpu.benchmark.memory', {
                  peak: formatGiB(result.peak_memory_bytes),
                  limit: formatGiB(result.memory_limit_bytes),
                })
              : null}{' '}
            {result.truncated ? t('cpu.benchmark.truncated') : null}{' '}
            <RelativeTime at={result.ran_at} />
          </p>
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          {range
            ? t('cpu.benchmark.onlyEstimate', { range })
            : t('cpu.benchmark.none')}
        </p>
      )}
    </section>
  )
}
