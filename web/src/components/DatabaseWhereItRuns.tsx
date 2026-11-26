import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CloudGlassIcon, ServerGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { useNodeListOptional } from '../queries/nodes'
import { ConnectHostedDialog } from './ConnectHostedDialog'

const OPTION =
  'flex min-h-14 flex-1 flex-col items-start justify-center gap-0.5 rounded-lg border px-3 py-2 text-left text-sm transition-colors motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:cursor-not-allowed disabled:opacity-60'

// First step of the create flow: where the database runs. Only real choices
// are enabled; cloud stays disabled until a cloud connection can provision.
export function DatabaseWhereItRuns() {
  const { t } = useTranslation('databases')
  const nodes = useNodeListOptional().data ?? []
  const multiNode = nodes.length > 1
  const [where, setWhere] = useState<'local' | 'node'>('local')
  const groupId = useId()
  const pick = (id: 'local' | 'node', disabled = false) =>
    cn(
      OPTION,
      disabled
        ? 'border-border'
        : where === id
          ? 'border-ring bg-tone-accent-soft'
          : 'border-border hover:bg-muted/60',
    )
  return (
    <div className="flex flex-col gap-2">
      <h3 id={groupId} className="text-sm font-medium">
        {t('where.title')}
      </h3>
      <div
        role="radiogroup"
        aria-labelledby={groupId}
        className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4"
      >
        <button
          type="button"
          role="radio"
          aria-checked={where === 'local'}
          className={pick('local')}
          onClick={() => {
            setWhere('local')
          }}
        >
          <span className="flex items-center gap-1.5 font-medium">
            <ServerGlassIcon size="1.2em" aria-hidden="true" />
            {t('where.local')}
          </span>
          <span className="text-xs text-muted-foreground">
            {t('where.localHint')}
          </span>
        </button>
        <button
          type="button"
          role="radio"
          aria-checked={where === 'node'}
          disabled={!multiNode}
          className={pick('node', !multiNode)}
          onClick={() => {
            setWhere('node')
          }}
        >
          <span className="flex items-center gap-1.5 font-medium">
            <ServerGlassIcon size="1.2em" aria-hidden="true" />
            {t('where.node')}
          </span>
          <span className="text-xs text-muted-foreground">
            {multiNode
              ? t('where.nodeHint', { count: nodes.length })
              : t('where.nodeNone')}
          </span>
        </button>
        <button
          type="button"
          role="radio"
          aria-checked={false}
          disabled
          className={pick('local', true)}
        >
          <span className="flex items-center gap-1.5 font-medium">
            <CloudGlassIcon size="1.2em" aria-hidden="true" />
            {t('where.cloud')}
          </span>
          <span className="text-xs text-muted-foreground">
            {t('where.cloudSoon')}
          </span>
        </button>
        <div className={cn(OPTION, 'border-border')}>
          <span className="flex items-center gap-1.5 font-medium">
            <CloudGlassIcon size="1.2em" aria-hidden="true" />
            {t('where.hosted')}
          </span>
          <ConnectHostedDialog compact />
        </div>
      </div>
      {where === 'node' && multiNode ? (
        <p className="text-xs text-muted-foreground">{t('where.nodeNext')}</p>
      ) : null}
    </div>
  )
}
