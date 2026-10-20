import { ArrowSquareOutIcon } from '@phosphor-icons/react/dist/ssr'
import { buttonVariants } from '@/components/ui/button'
import { useObservabilitySettingsOptional } from '../queries/observabilitySettings'

// Small addition next to the existing metrics dashboard headers
// (MetricsDashboard.tsx, NodeMetricsDashboard.tsx): a plain external
// link to wherever the operator already runs Grafana, set on
// Settings > Observability (ObservabilitySettingsCard.tsx). Renders
// nothing until that URL is configured, and nothing on a failed/loading
// fetch, since a broken link-out must never block or clutter a page
// whose real job is the charts.
export function ViewInGrafanaLink() {
  const { data } = useObservabilitySettingsOptional()
  const url = data?.external_dashboard_url
  if (!url) return null

  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className={buttonVariants({ variant: 'outline', size: 'sm' })}
    >
      View in Grafana
      <ArrowSquareOutIcon />
    </a>
  )
}
