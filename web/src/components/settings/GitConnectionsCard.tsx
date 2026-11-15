import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, UsersGlassIcon } from '@thesvg/ui-react'
import { isTokenHost } from '../../lib/gitTokenGuides'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { useGitConnections } from '../../queries/gitConnections'
import type { GitConnectionProvider } from '../../types/gitConnections'
import { AddGitConnectionDialog } from './AddGitConnectionDialog'
import { GitAccessMethods } from './git/GitAccessMethods'
import { GitAccessPanel } from './git/GitAccessPanel'
import { GitNextSteps } from './git/GitNextSteps'

const ConnectWithTokenDialog = lazy(
  () => import('../gitConnections/ConnectWithTokenDialog'),
)

// The provider page body: ways to connect, one access panel per connected
// account, and what to do next. Connecting never replaces an account.
export function GitConnectionsCard({
  provider,
}: Readonly<{ provider: GitConnectionProvider }>) {
  const { t } = useTranslation('settings')
  const { data: connections } = useGitConnections(provider)
  const [adding, setAdding] = useState(false)
  const [tokenOpen, setTokenOpen] = useState(false)
  const tokenHost = isTokenHost(provider) ? provider : undefined
  const authorizedCount = connections.filter((c) => c.authorized).length

  function startToken() {
    if (tokenHost) setTokenOpen(true)
    else setAdding(true)
  }

  return (
    <div className="space-y-6">
      <GitAccessMethods
        provider={provider}
        connections={connections}
        onOAuth={() => setAdding(true)}
        onToken={startToken}
      />
      {connections.length === 0 ? (
        <EmptyState
          compact
          icon={<UsersGlassIcon className="size-6" />}
          title={t('gitConnections.empty.title')}
          description={t(`gitConnections.empty.description.${provider}`)}
          action={
            <Button type="button" size="sm" onClick={() => setAdding(true)}>
              <PlusGlassIcon className="size-5" aria-hidden="true" />
              {t('gitConnections.add.first')}
            </Button>
          }
        />
      ) : (
        <section className="space-y-4">
          {connections.map((c) => (
            <GitAccessPanel
              key={c.id}
              provider={provider}
              connection={c}
              canSetDefault={authorizedCount > 1}
              onAddAnother={() => setAdding(true)}
            />
          ))}
          <p className="text-xs text-muted-foreground">
            {t('gitConnections.scopeNote')}
          </p>
        </section>
      )}
      <GitNextSteps />
      {tokenHost && tokenOpen ? (
        <Suspense fallback={null}>
          <ConnectWithTokenDialog
            provider={tokenHost}
            open
            onOpenChange={setTokenOpen}
          />
        </Suspense>
      ) : null}
      <AddGitConnectionDialog
        provider={provider}
        existing={connections}
        open={adding}
        onOpenChange={setAdding}
        onUseToken={
          tokenHost
            ? () => {
                setAdding(false)
                setTokenOpen(true)
              }
            : undefined
        }
      />
    </div>
  )
}
