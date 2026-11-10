import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { GuidedEmptyState } from './GuidedEmptyState'

export const FIREWALL_CLI_ARGS = 'firewall allow --port 443 --protocol tcp'

/** FirewallEmptyState guides a truly empty firewall list to its first item. */
export function FirewallEmptyState({ total }: { total: number }) {
  const { t } = useTranslation('statePlaceholders')
  return (
    <GuidedEmptyState
      total={total}
      icon={<ShieldGlassIcon size="1.4em" />}
      title={t('guided.firewall.title')}
      description={t('guided.firewall.body')}
      cliArgs={FIREWALL_CLI_ARGS}
      helpPath="/firewall"
      action={
        <Button
          size="sm"
          render={<Link to="/firewall/rules/new" />}
          nativeButton={false}
        >
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('guided.firewall.action')}
        </Button>
      }
    />
  )
}
