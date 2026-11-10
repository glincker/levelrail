import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArchiveGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { GuidedEmptyState } from './GuidedEmptyState'

export const BACKUPS_CLI_ARGS = 'backups trigger app-db'

/** BackupsEmptyState guides a truly empty backups list to its first item. */
export function BackupsEmptyState({ total }: { total: number }) {
  const { t } = useTranslation('statePlaceholders')
  return (
    <GuidedEmptyState
      total={total}
      icon={<ArchiveGlassIcon size="1.4em" />}
      title={t('guided.backups.title')}
      description={t('guided.backups.body')}
      cliArgs={BACKUPS_CLI_ARGS}
      helpPath="/backups"
      action={
        <Button
          size="sm"
          render={<Link to="/settings/backup-targets" />}
          nativeButton={false}
        >
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('guided.backups.action')}
        </Button>
      }
    />
  )
}
