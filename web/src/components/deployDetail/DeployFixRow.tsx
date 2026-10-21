import { Link } from '@tanstack/react-router'
import { WrenchIcon } from '@phosphor-icons/react/dist/ssr'
import { buttonVariants } from '@/components/ui/button'
import { deployFixLinks } from '../../lib/deployFixLinks'
import type { DeployStage } from '../../lib/deployStages'
import { useDeployDetailT } from './useDeployDetailT'

/** DeployFixRow links to the setting most likely behind the failure; redeploying stays with the actions beside it. */
export function DeployFixRow({
  appName,
  stage,
  causeId,
}: {
  appName: string
  stage: DeployStage['key']
  causeId?: string
}) {
  const { t } = useDeployDetailT()
  const links = deployFixLinks(stage, causeId)
  return (
    <div className="space-y-1.5" data-testid="deploy-fix-row">
      <p className="flex items-center gap-1 text-xs font-medium text-muted-foreground uppercase tracking-wide">
        <WrenchIcon className="size-3.5" aria-hidden="true" />
        {t('fix.title')}
      </p>
      <p className="text-sm text-muted-foreground">{t('fix.hint')}</p>
      <div className="flex flex-wrap gap-2">
        {links.map((link) => (
          <Link
            key={link.to}
            to={link.to}
            params={{ name: appName }}
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            {t(`fix.link.${link.labelKey}`)}
          </Link>
        ))}
      </div>
    </div>
  )
}
