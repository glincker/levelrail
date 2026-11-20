import { useState } from 'react'
import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import type { DeployStepEvent } from '../../hooks/useDeployStepStream'
import type { BuildStageName } from '../../lib/buildStages'
import { StageLogPanel } from './StageLogPanel'
import { StageOverview } from './StageOverview'
import { useBuildStages, useRefetchOnBuildSteps } from './useBuildStages'

/** Self-contained waterfall and stage log for compact hosts. */
export function BuildStagesPanel({
  appName,
  deployId,
  steps = [],
}: {
  appName: string
  deployId: string
  steps?: DeployStepEvent[]
}) {
  const { t } = useDeployDetailT()
  const [selected, setSelected] = useState<BuildStageName | null>(null)
  const { data } = useBuildStages(appName, deployId)
  useRefetchOnBuildSteps(appName, deployId, steps)
  const finished =
    data !== undefined && data.status !== 'running' && data.status !== 'queued'
  return (
    <div className="space-y-3">
      <StageOverview
        appName={appName}
        deployId={deployId}
        selected={selected}
        onSelect={setSelected}
      />
      {data && data.stages.length > 0 ? (
        <>
          {selected ? (
            <StageLogPanel
              key={selected}
              appName={appName}
              deployId={deployId}
              stage={selected}
              finished={finished}
            />
          ) : (
            <p className="text-xs text-muted-foreground">
              {t('buildStages.pickHint')}
            </p>
          )}
        </>
      ) : null}
    </div>
  )
}
