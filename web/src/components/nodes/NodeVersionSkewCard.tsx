import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useNodeVersionSkew } from '../../queries/nodeInsight'

// Only appears when at least one agent is behind, so a current fleet stays quiet.
export function NodeVersionSkewCard() {
  const { t } = useTranslation('nodes')
  const { data } = useNodeVersionSkew()
  if (!data || data.behind === 0) return null

  return (
    <Card id="node-version-skew">
      <CardHeader>
        <CardTitle>
          {t('skew.title', {
            count: data.behind,
            version: data.control_plane_version,
          })}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <p className="text-muted-foreground">{t('skew.description')}</p>
        <ol className="list-decimal space-y-1 pl-5">
          {data.plan.map((s) => (
            <li key={s.node_id}>
              <Link
                to="/nodes/$id"
                params={{ id: s.node_id }}
                className="font-medium underline-offset-2 hover:underline"
              >
                {s.name}
              </Link>{' '}
              <span className="text-muted-foreground">
                {t('skew.step', { from: s.from || '?', to: s.to })}
                {s.note === 'cordoned' ? `, ${t('skew.notes.cordoned')}` : ''}
                {s.note === 'offline' ? `, ${t('skew.notes.offline')}` : ''}
              </span>
            </li>
          ))}
        </ol>
        <p className="text-xs text-muted-foreground">{t('skew.howTo')}</p>
      </CardContent>
    </Card>
  )
}
