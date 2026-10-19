import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { routeErrorMessage } from '../../lib/apiError'
import {
  usePendingJoinTokens,
  useRevokeJoinTokens,
} from '../../queries/nodeJoinTokens'

export function PendingJoinTokensCard() {
  const { t } = useTranslation('nodes')
  const { data } = usePendingJoinTokens()
  const revoke = useRevokeJoinTokens()
  if (!data || data.length === 0) return null

  const run = (id?: string) =>
    revoke.mutate(id, {
      onSuccess: (res) =>
        toast.add({
          title: t('joinTokens.revoked', { count: res.revoked }),
          type: 'success',
        }),
      onError: (err) =>
        toast.add({ title: routeErrorMessage(err), type: 'error' }),
    })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('joinTokens.title', { count: data.length })}</CardTitle>
        <CardDescription>{t('joinTokens.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <ul className="space-y-2">
          {data.map((tok) => (
            <li
              key={tok.id}
              className="flex flex-wrap items-center justify-between gap-2 text-sm"
            >
              <span>
                <span className="font-mono text-xs">{tok.id}</span>{' '}
                <span className="text-muted-foreground">
                  {t('joinTokens.expires', {
                    when: new Date(tok.expires_at).toLocaleTimeString(),
                  })}
                </span>
              </span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={revoke.isPending}
                onClick={() => run(tok.id)}
              >
                {t('joinTokens.revoke')}
              </Button>
            </li>
          ))}
        </ul>
        {data.length > 1 ? (
          <Button
            type="button"
            size="sm"
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() => run()}
          >
            {t('joinTokens.revokeAll')}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  )
}
