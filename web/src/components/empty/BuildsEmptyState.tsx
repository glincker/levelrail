import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { HammerGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { GuidedEmptyState } from './GuidedEmptyState'

export const BUILDS_CLI_ARGS =
  'build detect --repo-url https://github.com/you/app'

/** BuildsEmptyState guides a truly empty builds list to its first item. */
export function BuildsEmptyState({ total }: { total: number }) {
  const { t } = useTranslation('statePlaceholders')
  return (
    <GuidedEmptyState
      total={total}
      icon={<HammerGlassIcon size="1.4em" />}
      title={t('guided.builds.title')}
      description={t('guided.builds.body')}
      cliArgs={BUILDS_CLI_ARGS}
      helpPath="/build-center"
      action={
        <Button size="sm" render={<Link to="/apps/new" />} nativeButton={false}>
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('guided.builds.action')}
        </Button>
      }
    />
  )
}
