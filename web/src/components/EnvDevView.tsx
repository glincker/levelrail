import { useState } from 'react'
import { ArrowsClockwiseIcon, CodeIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import type { SecretKeyState } from '../queries/secrets'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { parseEnvBlock } from '../lib/envParse'

function formatEnvText(env: Record<string, string>): string {
  return Object.keys(env)
    .sort()
    .map((key) => `${key}=${env[key]}`)
    .join('\n')
}

// A raw, auto-formatted text editor for plain env vars, the Coolify-
// style "developer view" alternative to EnvEditor's row-by-row table.
// Secret keys are listed alongside for visibility only, never editable
// here: Levelrail's secrets are write-only by design (no GET ever
// returns a value), so there is no safe way to represent a secret's
// current value as an editable line the way an unlocked Coolify
// variable can be. Changing a secret's value always goes through
// SecretsEditor's dedicated form, which has its own lock/confirm flow.
export function EnvDevView({
  app,
  secretKeys,
}: {
  app: AppDetail
  secretKeys: SecretKeyState[]
}) {
  const updateApp = useUpdateApp(app.name)
  const [text, setText] = useState(() => formatEnvText(app.env ?? {}))

  const handleFormat = () => {
    const parsed = parseEnvBlock(text)
    const env: Record<string, string> = {}
    for (const { key, value } of parsed) {
      env[key] = value
    }
    setText(formatEnvText(env))
  }

  const handleSave = () => {
    const parsed = parseEnvBlock(text)
    const env: Record<string, string> = {}
    for (const { key, value } of parsed) {
      env[key] = value
    }
    updateApp.mutate(
      { ...app, env },
      {
        onSuccess: () => {
          toast.add({ title: 'Variables saved.', type: 'success' })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CodeIcon className="size-4" />
          Developer view
        </CardTitle>
        <CardDescription>
          Raw KEY=value editing for plain env vars. Secret keys are listed
          below for visibility only: change a secret&apos;s value through
          the Secrets card, this view never reads or writes one.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <Textarea
          value={text}
          onChange={(e) => {
            setText(e.target.value)
          }}
          className="min-h-48 font-mono"
          spellCheck={false}
        />
        {secretKeys.length > 0 ? (
          <div className="rounded-md border border-border p-2 text-sm text-muted-foreground">
            {secretKeys.map((k) => (
              <div key={k.key} className="font-mono">
                {k.key} (write-only{k.locked ? ', locked' : ''})
              </div>
            ))}
          </div>
        ) : null}
        <div className="flex items-center gap-2">
          <Button type="button" variant="outline" size="sm" onClick={handleFormat}>
            <ArrowsClockwiseIcon />
            Format
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={updateApp.isPending}
            onClick={handleSave}
          >
            {updateApp.isPending ? 'Saving...' : 'Save variables'}
          </Button>
        </div>
        {updateApp.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{updateApp.error.message}</AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  )
}
