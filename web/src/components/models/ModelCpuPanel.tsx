import { InfoGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { formatCpus, formatMemoryMiB } from '../../lib/modelsCpu'
import { useCpuFit } from '../../queries/modelsCpu'
import type { ModelResource } from '../../types/models'
import { ModelBenchmarkCard } from './ModelBenchmarkCard'
import { ModelDownloadProgress } from './ModelDownloadProgress'

// Overview section of a CPU model: its hard caps, the weights download,
// the benchmark next to the estimate and what idle unload does.
export function ModelCpuPanel({ model }: { model: ModelResource }) {
  const { t } = useTranslation('models')
  const fit = useCpuFit(
    {
      compute: 'cpu',
      model: model.model,
      context_length: model.context_length,
      cpus: model.cpu_limit,
      memory_mib: model.memory_limit_mib,
    },
    true,
  )
  const showDownload =
    model.download !== undefined && model.status.reason === 'Downloading'
  return (
    <div className="space-y-6">
      <Alert>
        <InfoGlassIcon
          className="text-tone-info"
          size="1.4em"
          aria-hidden="true"
        />
        <AlertTitle>{t('cpu.detail.capsTitle')}</AlertTitle>
        <AlertDescription>
          {t('cpu.detail.caps', {
            cpus: formatCpus((model.cpu_limit ?? 0) * 1000),
            memory: formatMemoryMiB(model.memory_limit_mib ?? 0),
          })}
        </AlertDescription>
      </Alert>
      {showDownload && model.download ? (
        <ModelDownloadProgress download={model.download} />
      ) : null}
      <ModelBenchmarkCard
        modelName={model.name}
        ready={model.status.ready}
        benchmark={model.benchmark}
        estimate={fit.data}
      />
      <p className="text-xs text-muted-foreground">
        {t('cpu.detail.idleExplain')}
      </p>
    </div>
  )
}
