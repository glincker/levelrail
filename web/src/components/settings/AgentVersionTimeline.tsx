import { useTranslation } from 'react-i18next'
import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr'
import { formatRelative } from '@/components/kit/formatRelative'
import type { AgentVersionChange } from '../../queries/upgradeHistory'

export function AgentVersionTimeline({
  changes,
}: {
  changes: AgentVersionChange[]
}) {
  const { t } = useTranslation('updates')
  if (changes.length === 0) return null
  return (
    <section
      aria-label={t('upgradeHistory.agents.title')}
      className="space-y-2"
    >
      <h3 className="text-sm font-medium text-foreground">
        {t('upgradeHistory.agents.title')}
      </h3>
      <ul className="divide-y rounded-md border">
        {changes.map((c) => (
          <li
            key={`${c.node_id}-${c.observed_at}-${c.to_version}`}
            className="flex flex-wrap items-center gap-2 px-3 py-2 text-xs"
          >
            <span className="font-medium text-foreground">
              {c.node_name || c.node_id}
            </span>
            {c.from_version !== '' ? (
              <>
                <span className="font-mono">{c.from_version}</span>
                <ArrowRightIcon className="size-3.5 text-muted-foreground" />
              </>
            ) : (
              <span className="text-muted-foreground">
                {t('upgradeHistory.agents.first')}
              </span>
            )}
            <span className="font-mono">{c.to_version}</span>
            <time
              dateTime={c.observed_at}
              title={new Date(c.observed_at).toLocaleString()}
              className="text-muted-foreground"
            >
              {formatRelative(c.observed_at)}
            </time>
          </li>
        ))}
      </ul>
    </section>
  )
}
