import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { teamEventsQueryOptions } from '../../queries/teams'

export function TeamEventsCard({ teamId }: { teamId: string }) {
  const { t } = useTranslation('teams')
  const { data: events = [] } = useQuery(teamEventsQueryOptions(teamId, true))
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('events.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        {events.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('events.empty')}</p>
        ) : (
          <ul className="space-y-1 text-sm">
            {events.map((e) => (
              <li key={e.id} className="flex flex-wrap gap-x-2">
                <time className="text-muted-foreground" dateTime={e.created_at}>
                  {new Date(e.created_at).toLocaleString()}
                </time>
                <span className="font-medium">{e.actor_name}</span>
                <code className="text-xs">{e.action}</code>
                <span>{e.subject}</span>
                <span className="text-muted-foreground">{e.detail}</span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
