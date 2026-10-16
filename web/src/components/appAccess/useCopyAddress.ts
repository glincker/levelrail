import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'

/** Copy handler that flips the icon and announces "Copied" through the toast. */
export function useCopyAddress() {
  const { t } = useTranslation('appAccess')
  const { copied, copy } = useCopyToClipboard()
  return {
    copied,
    copy: (value: string) => {
      copy(value)
      toast.add({ title: t('actions.copied'), type: 'success' })
    },
  }
}
