import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'

export type CopyDoneKey =
  | 'viewer.grid.copied'
  | 'viewer.grid.copied_csv'
  | 'viewer.grid.copied_json'
  | 'viewer.grid.copied_insert'
  | 'viewer.structure.ddlCopied'

export function useCopy(): (text: string, doneKey?: CopyDoneKey) => void {
  const { t } = useTranslation('databases')
  return useCallback(
    (text, doneKey = 'viewer.grid.copied') => {
      void navigator.clipboard
        .writeText(text)
        .then(() => {
          toast.add({ title: t(doneKey), type: 'success' })
        })
        .catch(() => {
          toast.add({ title: t('viewer.grid.copyFailed'), type: 'error' })
        })
    },
    [t],
  )
}
