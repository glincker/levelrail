import { InfoTip, StatusPill } from '@/components/kit'
import {
  FIT_VERDICT_LABEL,
  FIT_VERDICT_TONE,
  fitSourceLabel,
} from '../../lib/modelFit'
import type { FitReport, NodeFit } from '../../types/modelFit'

export function FitPill({ node }: { node: NodeFit }) {
  return (
    <span className="inline-flex items-center gap-1">
      <StatusPill
        tone={FIT_VERDICT_TONE[node.verdict]}
        label={FIT_VERDICT_LABEL[node.verdict]}
        size="sm"
      />
      <InfoTip label={`How ${node.name} was estimated`}>
        <div className="space-y-1.5 text-xs">
          <p className="font-mono">{node.arithmetic}</p>
          <p className="text-muted-foreground">{fitSourceLabel(node)}.</p>
          {node.context_assumed ? (
            <p className="text-muted-foreground">
              No context length set: {node.context_tokens} tokens assumed.
            </p>
          ) : null}
          {node.reserved_bytes > 0 ? (
            <p className="text-muted-foreground">
              Includes VRAM reserved for models still loading on this node.
            </p>
          ) : null}
          {node.reason ? <p>{node.reason}</p> : null}
          {node.suggestions.map((s) => (
            <p key={s}>{s}</p>
          ))}
        </div>
      </InfoTip>
    </span>
  )
}

// Node rows with a verdict each. `highlight` picks the row that matters
// (the node the form or the model is on).
export function FitNodeList({
  report,
  highlight,
}: {
  report: FitReport
  highlight: (n: NodeFit) => boolean
}) {
  if (report.nodes.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No GPU node has reported yet, so fit cannot be estimated.
      </p>
    )
  }
  return (
    <div className="space-y-1.5">
      <ul className="space-y-1">
        {report.nodes.map((n) => (
          <li
            key={n.node_id || 'local'}
            className="flex items-center justify-between gap-2 text-xs"
          >
            <span
              className={
                highlight(n)
                  ? 'font-medium text-foreground'
                  : 'text-muted-foreground'
              }
            >
              {n.name}
              {n.current ? ' (current)' : ''}
              {n.eligible ? '' : ' (not schedulable)'}
            </span>
            <FitPill node={n} />
          </li>
        ))}
      </ul>
      <p className="text-[11px] text-muted-foreground">
        Estimate only. {report.note}
      </p>
    </div>
  )
}
