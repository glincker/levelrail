import { CoinsIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { HelpLink } from './HelpLink'
import { useCostEstimate } from '../queries/costEstimate'
import type { CostEstimateProvider } from '../types/costEstimate'

// Purely informational, like ResourceRecommendationCard next to it:
// this never changes a resource limit, it only shows what the app's
// own CPU/memory envelope would cost under a few illustrative
// reference providers. ESTIMATE ONLY, never a real bill; see
// docs/cost-estimate.md (linked via the HelpLink below) for the
// formula and how an operator corrects the rates for their own region.
export function CostEstimateCard({ appName }: { appName: string }) {
  const { data, isLoading, isError } = useCostEstimate(appName)

  if (isLoading) {
    return null
  }
  if (isError || !data) {
    return (
      <Card>
        <CardContent className="pt-6">
          <p className="text-sm text-muted-foreground">
            Could not load a cost estimate right now.
          </p>
        </CardContent>
      </Card>
    )
  }

  const headline =
    data.providers.find((p) => p.key === 'vm') ?? data.providers[0]
  const hasSize =
    data.cpu_basis !== 'unavailable' || data.memory_basis !== 'unavailable'

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CoinsIcon className="size-4" />
          What this would cost elsewhere
          <HelpLink
            path="/cost-estimate"
            label="How this estimate is calculated"
          />
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {headline ? (
          <div>
            <p className="text-2xl font-semibold text-foreground">
              ~${headline.total_usd.toFixed(2)}/mo equivalent
            </p>
            <p className="text-xs text-muted-foreground">{headline.label}</p>
          </div>
        ) : null}

        {hasSize ? (
          <p className="text-sm text-muted-foreground">
            Based on {data.vcpu_cores.toFixed(2)} vCPU (
            <BasisLabel basis={data.cpu_basis} />) and{' '}
            {data.memory_gib.toFixed(2)} GiB memory (
            <BasisLabel basis={data.memory_basis} />
            ).
          </p>
        ) : (
          <p className="text-sm text-muted-foreground">
            No declared resource limits and not enough usage history yet to size
            this estimate; showing each provider's minimum monthly charge.
          </p>
        )}

        <div className="space-y-2">
          {data.providers.map((p) => (
            <ProviderRow key={p.key} provider={p} />
          ))}
        </div>

        <p className="text-xs text-muted-foreground">
          This is an estimate, not a real bill: it compares this app's resource
          envelope against illustrative reference rates, never a live quote.
        </p>
      </CardContent>
    </Card>
  )
}

function BasisLabel({ basis }: { basis: string }) {
  if (basis === 'declared') return <span>declared</span>
  if (basis === 'observed') return <span>observed usage</span>
  return <span>no data</span>
}

function ProviderRow({ provider }: { provider: CostEstimateProvider }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
      <div>
        <p className="text-sm font-medium text-foreground">{provider.label}</p>
        <p className="text-xs text-muted-foreground">
          cpu ${provider.cpu_cost_usd.toFixed(2)} + memory $
          {provider.memory_cost_usd.toFixed(2)}
        </p>
      </div>
      <div className="flex items-center gap-2">
        {provider.minimum_applied ? (
          <Badge variant="muted">minimum</Badge>
        ) : null}
        <p className="text-sm font-semibold text-foreground">
          ${provider.total_usd.toFixed(2)}/mo
        </p>
      </div>
    </div>
  )
}
