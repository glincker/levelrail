import {
  ArrowsClockwiseIcon,
  DotsThreeIcon,
  ExportIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  ActionMenu,
  AnimatedNumber,
  InfoTip,
  StatusPill,
} from '@/components/kit'
import { Button } from '@/components/ui/button'
import { ALGORITHM_OPTIONS } from '../../lib/loadBalancer'
import type { LoadBalancerAlgorithm } from '../../queries/appLoadBalancer'
import { UNSUPPORTED_HINT } from './adminActions'
import { StateGlyphIcon } from './StateGlyphIcon'
import type { PoolRollup } from './rollup'

interface Props {
  rollup: PoolRollup
  algorithm: LoadBalancerAlgorithm
  checkSupported: boolean
  checking: boolean
  cooldown: number
  checkNote?: string
  onCheck: () => void
  onExport: () => void
  onRemove: () => void
}

const GLYPH = {
  balancing: 'ok',
  degraded: 'warn',
  down: 'down',
  idle: 'unknown',
} as const

export function LbHeader({
  rollup,
  algorithm,
  checkSupported,
  checking,
  cooldown,
  checkNote,
  onCheck,
  onExport,
  onRemove,
}: Props) {
  const algo = ALGORITHM_OPTIONS.find((o) => o.value === algorithm)
  return (
    <header className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <StatusPill
        tone={rollup.tone}
        label={rollup.label}
        live={rollup.level === 'balancing'}
        icon={<StateGlyphIcon glyph={GLYPH[rollup.level]} />}
      />
      <p className="text-lg font-semibold" aria-live="polite">
        <AnimatedNumber value={rollup.healthy} /> of{' '}
        <AnimatedNumber value={rollup.total} /> healthy
      </p>
      <span className="inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs">
        {algo?.label ?? algorithm}
        <InfoTip label="About this algorithm">{algo?.description}</InfoTip>
      </span>
      <div className="ml-auto flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!checkSupported || checking || cooldown > 0}
          title={checkSupported ? undefined : UNSUPPORTED_HINT}
          onClick={onCheck}
        >
          <ArrowsClockwiseIcon
            data-icon="inline-start"
            className={checking ? 'animate-spin' : undefined}
          />
          Check now{cooldown > 0 ? ` (${cooldown}s)` : ''}
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={onExport}>
          <ExportIcon data-icon="inline-start" />
          Export
        </Button>
        <ActionMenu
          trigger={
            <Button
              type="button"
              size="icon"
              variant="ghost"
              aria-label="More actions"
            >
              <DotsThreeIcon />
            </Button>
          }
          items={[
            {
              id: 'remove',
              label: 'Remove load balancer',
              description: 'Domains go back to a single upstream',
              icon: <TrashIcon />,
              tone: 'danger',
              onSelect: onRemove,
            },
          ]}
        />
      </div>
      {checkNote ? (
        <p className="basis-full text-xs text-muted-foreground">{checkNote}</p>
      ) : null}
    </header>
  )
}
