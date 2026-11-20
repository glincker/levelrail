import { useTranslation } from 'react-i18next'
import { ResourceTiles } from '../overview/ResourceTiles'
import { useResourceReading } from '../overview/useResourceReading'
import { AppCostChip } from '../usage/AppCostChip'
import type { AppDetail } from '../../types/appDetail'
import { StatTile } from './StatTile'

/** Live CPU and memory tiles with usage bars, plus replicas, in one strip. */
export function ResourcesSummary({ app }: Readonly<{ app: AppDetail }>) {
  const { t } = useTranslation('appPages')
  const reading = useResourceReading(app)
  return (
    <div className="space-y-3">
      <section
        aria-label={t('resources.summary')}
        className="grid grid-cols-1 gap-3 sm:grid-cols-3"
      >
        <ResourceTiles reading={reading} />
        <StatTile
          label={t('resources.replicas')}
          value={app.replicas}
          hint={<AppCostChip appName={app.name} />}
        />
      </section>
    </div>
  )
}
