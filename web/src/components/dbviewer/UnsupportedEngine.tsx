import { useTranslation } from 'react-i18next'
import { DatabaseIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/ui/empty-state'

export function UnsupportedEngine({ engine }: { engine: string }) {
  const { t } = useTranslation('databases')
  return (
    <EmptyState
      icon={<DatabaseIcon className="size-5" />}
      title={t('viewer.explorer.emptyTitle')}
      description={t('viewer.unsupported', { engine })}
    />
  )
}
