import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'

export function UnsupportedEngine({ engine }: { engine: string }) {
  const { t } = useTranslation('databases')
  return (
    <EmptyState
      icon={<DatabaseGlassIcon className="size-6" />}
      title={t('viewer.explorer.emptyTitle')}
      description={t('viewer.unsupported', { engine })}
    />
  )
}
