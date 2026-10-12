import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr'
import { useApp } from '../../../queries/apps'
import { EnvironmentEditor } from '../../../components/env/EnvironmentEditor'
import { EnvActivityPanel } from '../../../components/EnvActivityPanel'
import { PreviewEnvOverridesEditor } from '../../../components/PreviewEnvOverridesEditor'
import { BranchEnvOverridesEditor } from '../../../components/BranchEnvOverridesEditor'
import { AppEnvCompareCard } from '../../../components/AppEnvCompareCard'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

// Reads app data from the query cache the parent layout route's loader primed.
export const Route = createFileRoute('/apps/$name/environment')({
  component: EnvironmentSection,
})

function EnvironmentSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { t } = useTranslation('envEditor')

  return (
    <div className="space-y-6">
      <EnvironmentEditor app={app} />
      <EnvActivityPanel appName={app.name} />
      <Collapsible>
        <CollapsibleTrigger className="group/advanced flex items-start justify-between gap-3 py-1">
          <span className="space-y-0.5">
            <span className="block text-sm font-medium">
              {t('advanced.title')}
            </span>
            <span className="block text-xs text-muted-foreground">
              {t('advanced.description')}
            </span>
          </span>
          <CaretDownIcon
            aria-hidden
            className="mt-1 size-4 transition-transform group-data-[panel-open]/advanced:rotate-180"
          />
        </CollapsibleTrigger>
        <CollapsiblePanel className="space-y-6 pt-4">
          <PreviewEnvOverridesEditor
            appName={app.name}
            overrides={app.preview_env_overrides}
          />
          <BranchEnvOverridesEditor appName={app.name} />
          <AppEnvCompareCard app={app} />
        </CollapsiblePanel>
      </Collapsible>
    </div>
  )
}
