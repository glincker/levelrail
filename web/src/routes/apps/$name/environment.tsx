import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { CodeIcon, ListIcon } from '@phosphor-icons/react/dist/ssr'
import { useApp } from '../../../queries/apps'
import { useSecretKeys } from '../../../queries/secrets'
import { EnvEditor } from '../../../components/EnvEditor'
import { EnvDevView } from '../../../components/EnvDevView'
import { SecretsEditor } from '../../../components/SecretsEditor'
import { Button } from '@/components/ui/button'

// Former "environment" tab, now a real deep-linkable route. Reads app
// data from the query cache the parent layout route's loader already
// primed.
export const Route = createFileRoute('/apps/$name/environment')({
  component: EnvironmentSection,
})

function EnvironmentSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const secretKeysQuery = useSecretKeys(name)
  const [view, setView] = useState<'normal' | 'dev'>('normal')

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            setView((v) => (v === 'normal' ? 'dev' : 'normal'))
          }}
        >
          {view === 'normal' ? (
            <>
              <CodeIcon />
              Developer view
            </>
          ) : (
            <>
              <ListIcon />
              Normal view
            </>
          )}
        </Button>
      </div>
      {view === 'dev' ? (
        <EnvDevView app={app} secretKeys={secretKeysQuery.data ?? []} />
      ) : (
        <EnvEditor app={app} />
      )}
      <SecretsEditor appName={app.name} />
    </div>
  )
}
