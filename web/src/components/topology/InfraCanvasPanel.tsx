import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type {
  ProjectTopologyEdge,
  ProjectTopologyNode,
} from '../../types/projectTopology'
import { type CanvasHealth, neighbours } from '../../lib/infraCanvasLayout'

interface Props {
  node: ProjectTopologyNode
  health: CanvasHealth
  nodes: Map<string, ProjectTopologyNode>
  edges: ProjectTopologyEdge[]
  onClose: () => void
  onSelect: (id: string) => void
}

const LINK_CLASS =
  'rounded-md px-2 py-1.5 text-sm font-medium text-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

export function InfraCanvasPanel({
  node,
  health,
  nodes,
  edges,
  onClose,
  onSelect,
}: Props) {
  const { t } = useTranslation('infraCanvas')
  const related = neighbours(edges, node.id)
  const ownerApp =
    node.kind === 'domain'
      ? related.map((r) => nodes.get(r.id)).find((n) => n?.kind === 'app')
      : undefined

  return (
    <aside
      aria-label={t('panel.label', { name: node.label })}
      className="absolute inset-y-0 right-0 z-10 flex w-72 max-w-full flex-col gap-4 overflow-y-auto border-l border-border bg-card p-4"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate font-medium text-foreground">{node.label}</p>
          <p className="text-xs text-muted-foreground">
            {t(`kind.${node.kind}`)}
            {node.status ? ` · ${t(`status.${health}`)}` : ''}
          </p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          onClick={onClose}
          aria-label={t('panel.close')}
        >
          <XGlassIcon className="size-5" aria-hidden="true" />
        </Button>
      </div>

      <section aria-label={t('panel.actions')} className="flex flex-col">
        {node.kind === 'app' ? (
          <>
            <Link
              to="/apps/$name/overview"
              params={{ name: node.label }}
              className={LINK_CLASS}
            >
              {t('panel.overview')}
            </Link>
            <Link
              to="/apps/$name/logs"
              params={{ name: node.label }}
              className={LINK_CLASS}
            >
              {t('panel.logs')}
            </Link>
            <Link
              to="/apps/$name/deploys"
              params={{ name: node.label }}
              className={LINK_CLASS}
            >
              {t('panel.deploys')}
            </Link>
            <Link
              to="/apps/$name/domains"
              params={{ name: node.label }}
              className={LINK_CLASS}
            >
              {t('panel.domains')}
            </Link>
          </>
        ) : null}
        {node.kind === 'database' ? (
          <>
            <Link
              to="/databases/$name/overview"
              params={{ name: node.label }}
              className={LINK_CLASS}
            >
              {t('panel.overview')}
            </Link>
            <Link
              to="/databases/$name/logs"
              params={{ name: node.label }}
              className={LINK_CLASS}
            >
              {t('panel.logs')}
            </Link>
            <Link
              to="/databases/$name/metrics"
              params={{ name: node.label }}
              className={LINK_CLASS}
            >
              {t('panel.metrics')}
            </Link>
          </>
        ) : null}
        {ownerApp ? (
          <Link
            to="/apps/$name/domains"
            params={{ name: ownerApp.label }}
            className={LINK_CLASS}
          >
            {t('panel.domains')}
          </Link>
        ) : null}
      </section>

      <section>
        <h2 className="mb-1 text-xs font-medium uppercase text-muted-foreground">
          {t('panel.connections')}
        </h2>
        {related.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('panel.noConnections')}
          </p>
        ) : (
          <ul className="flex flex-col">
            {related.map((r) => {
              const other = nodes.get(r.id)
              if (!other) return null
              return (
                <li key={`${r.id}:${r.kind}:${r.outgoing}`}>
                  <button
                    type="button"
                    onClick={() => onSelect(r.id)}
                    className="flex w-full flex-col rounded-md px-2 py-1.5 text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <span className="truncate text-sm text-foreground">
                      {other.label}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {t(`edge.${r.kind}`)}
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </section>
    </aside>
  )
}
