import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'

import type { ExistingVolumeInfo } from '../lib/apiError'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { Button } from './ui/button'

interface ExistingVolumePromptProps {
  volumes: ExistingVolumeInfo[]
  pending: boolean
  onChoose: (choice: 'reuse' | 'discard') => void
}

function formatSize(bytes: number): string {
  if (bytes < 0) return '?'
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KiB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`
}

// Shown when creating a database whose name still has a data volume from a
// deleted one: the operator picks reuse or discard instead of a silent reuse.
export function ExistingVolumePrompt({
  volumes,
  pending,
  onChoose,
}: ExistingVolumePromptProps) {
  const { t } = useTranslation('databases')
  const data = volumes.find((v) => v.name.endsWith('-data'))
  return (
    <Alert variant="destructive">
      <WarningIcon />
      <AlertTitle>{t('existingVolume.title')}</AlertTitle>
      <AlertDescription>
        <p>
          {t('existingVolume.description', {
            size: formatSize(data?.size_bytes ?? -1),
          })}
        </p>
        <div className="mt-2 flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={pending}
            onClick={() => onChoose('reuse')}
          >
            {pending ? t('existingVolume.working') : t('existingVolume.reuse')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="sm"
            disabled={pending}
            onClick={() => onChoose('discard')}
          >
            {t('existingVolume.discard')}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  )
}
