import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { GearIcon, TerminalWindowIcon } from '@phosphor-icons/react/dist/ssr'
import { CliConsole } from '../cliConsole/CliConsole'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'

export function DockConsoleTab() {
  const { t } = useTranslation()
  const experimental = useExperimentalFeatures()
  if (!experimental.includes('cli-console')) {
    return (
      <div className="flex h-full min-h-0 flex-col p-3">
        <EmptyState
          icon={<TerminalWindowIcon className="size-5" />}
          title={t('cliConsole.dock.offTitle')}
          description={t('cliConsole.dock.offDescription')}
          action={
            <Button size="sm" render={<Link to="/settings/beta-features" />}>
              <GearIcon />
              {t('dock.ai.enableInSettings')}
            </Button>
          }
          className="flex-1"
        />
      </div>
    )
  }
  return (
    <div className="flex h-full min-h-0 flex-col p-3">
      <CliConsole className="flex-1" />
    </div>
  )
}
