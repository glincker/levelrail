import { useTranslation } from 'react-i18next'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import { downloadPercent, formatGiB } from '../../lib/modelsCpu'
import type { ModelDownload } from '../../types/modelsCpu'

// Bytes done and total of the weights download the control plane performs
// for a CPU model; the bar stops moving with reduced motion.
export function ModelDownloadProgress({
  download,
  compact = false,
}: {
  download: ModelDownload
  compact?: boolean
}) {
  const { t } = useTranslation('models')
  const percent = downloadPercent(download.done_bytes, download.total_bytes)
  return (
    <Progress
      value={percent}
      aria-label={t('cpu.download.label')}
      className="gap-1.5 motion-reduce:[&_[data-slot=progress-indicator]]:transition-none"
    >
      {compact ? null : (
        <ProgressLabel>{t('cpu.download.label')}</ProgressLabel>
      )}
      <ProgressValue>
        {() =>
          t('cpu.download.value', {
            done: formatGiB(download.done_bytes),
            total: formatGiB(download.total_bytes),
            percent,
          })
        }
      </ProgressValue>
    </Progress>
  )
}
