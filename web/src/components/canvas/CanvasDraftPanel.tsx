import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { ProjectTopologyGraph } from '../../types/projectTopology'
import type { DraftOp } from '../../lib/canvasDraft'
import { ConnectForm } from './CanvasForms'

// Cap the connection list so a very large project never renders a long column.
const MAX_CONNECTIONS_SHOWN = 50

function opLabel(t: TFunction<'canvas'>, op: DraftOp): string {
  switch (op.type) {
    case 'add-app':
      return t('ops.addApp', { name: op.name, image: op.image })
    case 'add-database':
      return t('ops.addDatabase', { name: op.name, engine: op.engine })
    case 'connect':
      return t('ops.connect', { app: op.app, database: op.database })
    case 'disconnect':
      return t('ops.disconnect', { app: op.app, database: op.database })
  }
}

export function CanvasDraftPanel({
  graph,
  ops,
  canReview,
  onAddApp,
  onAddDatabase,
  onConnect,
  onDisconnect,
  onRemoveOp,
  onDiscard,
  onReview,
}: {
  graph: ProjectTopologyGraph
  ops: readonly DraftOp[]
  canReview: boolean
  onAddApp: () => void
  onAddDatabase: () => void
  onConnect: (app: string, database: string, envVar: string) => void
  onDisconnect: (app: string, database: string) => void
  onRemoveOp: (id: string) => void
  onDiscard: () => void
  onReview: () => void
}) {
  const { t } = useTranslation('canvas')
  const labelOf = new Map(graph.nodes.map((n) => [n.id, n.label]))
  const apps = graph.nodes.filter((n) => n.kind === 'app').map((n) => n.label)
  const databases = graph.nodes
    .filter((n) => n.kind === 'database')
    .map((n) => n.label)
  const bindings = graph.edges.filter((e) => e.kind === 'database_binding')
  const shown = bindings.slice(0, MAX_CONNECTIONS_SHOWN)

  return (
    <div className="space-y-5 text-sm">
      <div className="flex flex-wrap gap-2">
        <Button type="button" size="sm" variant="outline" onClick={onAddApp}>
          <PlusGlassIcon size="1.2em" aria-hidden="true" />
          {t('add.app')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={onAddDatabase}
        >
          <PlusGlassIcon size="1.2em" aria-hidden="true" />
          {t('add.database')}
        </Button>
      </div>

      <section aria-labelledby="canvas-connect-heading" className="space-y-2">
        <h2 id="canvas-connect-heading" className="font-medium">
          {t('connect.title')}
        </h2>
        <ConnectForm apps={apps} databases={databases} onConnect={onConnect} />
      </section>

      <section aria-labelledby="canvas-conn-heading" className="space-y-2">
        <h2 id="canvas-conn-heading" className="font-medium">
          {t('connections.title')}
        </h2>
        {bindings.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('connections.none')}
          </p>
        ) : (
          <ul className="space-y-1">
            {shown.map((e) => {
              const app = labelOf.get(e.from) ?? e.from
              const database = labelOf.get(e.to) ?? e.to
              return (
                <li
                  key={`${e.from}>${e.to}`}
                  className="flex items-center justify-between gap-2"
                >
                  <span className="min-w-0 truncate">
                    {app} {'->'} {database}
                  </span>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t('connections.remove', { app, database })}
                    onClick={() => onDisconnect(app, database)}
                  >
                    <XGlassIcon size="1.2em" aria-hidden="true" />
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
        {bindings.length > shown.length ? (
          <p className="text-xs text-muted-foreground">
            {t('connections.more', {
              shown: shown.length,
              total: bindings.length,
            })}
          </p>
        ) : null}
      </section>

      <section aria-labelledby="canvas-draft-heading" className="space-y-2">
        <h2 id="canvas-draft-heading" className="font-medium">
          {t('draft.title')}
        </h2>
        {ops.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t('draft.none')}</p>
        ) : (
          <>
            <p className="text-xs text-muted-foreground" aria-live="polite">
              {t('draft.count', { count: ops.length })}
            </p>
            <ul className="space-y-1">
              {ops.map((op) => {
                const label = opLabel(t, op)
                return (
                  <li
                    key={op.id}
                    className="flex items-center justify-between gap-2"
                  >
                    <span className="min-w-0 break-words">{label}</span>
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      aria-label={t('draft.remove', { label })}
                      onClick={() => onRemoveOp(op.id)}
                    >
                      <XGlassIcon size="1.2em" aria-hidden="true" />
                    </Button>
                  </li>
                )
              })}
            </ul>
          </>
        )}
        <div className="flex flex-wrap gap-2 pt-1">
          <Button
            type="button"
            size="sm"
            disabled={!canReview}
            onClick={onReview}
          >
            {t('draft.review')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={ops.length === 0}
            onClick={onDiscard}
          >
            {t('draft.discard')}
          </Button>
        </div>
      </section>

      <section className="rounded-lg border border-border p-3">
        <h2 className="text-xs font-medium">{t('limits.title')}</h2>
        <p className="mt-1 text-xs text-muted-foreground">{t('limits.body')}</p>
      </section>
    </div>
  )
}
