import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { KeyGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { buttonVariants } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { CreateTokenDialog } from '../CreateTokenDialog'
import { TokenTable } from '../TokenTable'
import { CardError, CardLoading } from './AccountCardStates'
import { tokenListQueryOptions } from '../../queries/tokens'

export function MyTokensCard() {
  const { t } = useTranslation('settings')
  const { data, isPending, isError, isFetching, refetch } = useQuery(
    tokenListQueryOptions(),
  )
  const active = (data ?? []).filter((tok) => !tok.revoked_at)
  return (
    <Card id="tokens">
      <CardHeader>
        <CardTitle>{t('accountPage.tokens.title')}</CardTitle>
        <CardDescription>{t('accountPage.tokens.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {isPending ? <CardLoading rows={3} /> : null}
        {isError ? (
          <CardError
            retrying={isFetching}
            onRetry={() => {
              void refetch()
            }}
          />
        ) : null}
        {data && active.length === 0 ? (
          <EmptyState
            compact
            icon={<KeyGlassIcon className="size-5.5" />}
            title={t('accountPage.tokens.emptyTitle')}
            description={t('accountPage.tokens.emptyBody')}
            action={<CreateTokenDialog />}
          />
        ) : null}
        {data && active.length > 0 ? (
          <>
            <TokenTable tokens={active} />
            <div className="flex flex-wrap items-center gap-3">
              <CreateTokenDialog />
              <Link
                to="/settings/tokens"
                className={buttonVariants({ variant: 'ghost', size: 'sm' })}
              >
                {t('accountPage.tokens.manage')}
              </Link>
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
