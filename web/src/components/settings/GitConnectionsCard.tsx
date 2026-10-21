import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusIcon, UsersThreeIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { useGitConnections } from '../../queries/gitConnections'
import type { GitConnectionProvider } from '../../types/gitConnections'
import { AddGitConnectionDialog } from './AddGitConnectionDialog'
import { GitConnectionRow } from './GitConnectionRow'

// Every account connected for one provider, with Add another account.
// Several GitLab groups, a personal and a work Bitbucket workspace, or two
// Gitea servers live side by side; connecting one never replaces another.
export function GitConnectionsCard({
  provider,
}: Readonly<{ provider: GitConnectionProvider }>) {
  const { t } = useTranslation('settings')
  const { data: connections } = useGitConnections(provider)
  const [adding, setAdding] = useState(false)
  const authorizedCount = connections.filter((c) => c.authorized).length

  return (
    <Card>
      <CardContent className="space-y-4">
        {connections.length === 0 ? (
          <EmptyState
            compact
            icon={<UsersThreeIcon className="size-5" />}
            title={t('gitConnections.empty.title')}
            description={t(`gitConnections.empty.description.${provider}`)}
            action={
              <Button type="button" size="sm" onClick={() => setAdding(true)}>
                <PlusIcon className="size-4" aria-hidden="true" />
                {t('gitConnections.add.first')}
              </Button>
            }
          />
        ) : (
          <>
            <ul className="divide-y divide-border">
              {connections.map((c) => (
                <GitConnectionRow
                  key={c.id}
                  connection={c}
                  canSetDefault={authorizedCount > 1}
                />
              ))}
            </ul>
            <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
              <p className="text-xs text-muted-foreground">
                {t('gitConnections.scopeNote')}
              </p>
              <Button type="button" size="sm" onClick={() => setAdding(true)}>
                <PlusIcon className="size-4" aria-hidden="true" />
                {t('gitConnections.add.another')}
              </Button>
            </div>
          </>
        )}
      </CardContent>
      <AddGitConnectionDialog
        provider={provider}
        existing={connections}
        open={adding}
        onOpenChange={setAdding}
      />
    </Card>
  )
}
