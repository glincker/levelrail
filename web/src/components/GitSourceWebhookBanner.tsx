import { useState } from 'react'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

// The "add this webhook by hand" banner CreateAppFromGitFields shows once
// a git source connects but the provider didn't auto-register a webhook
// (GitHub's own webhookError degrade path, or any provider through the
// generic connect fallback). Split out once the parent form crossed a
// comfortable single-file size, the same reasoning GitBuildSourceFields'
// own doc comment gives for its own split.
export function GitSourceWebhookBanner({
  webhookUrl,
  webhookSecret,
  webhookError,
}: {
  webhookUrl: string
  webhookSecret: string
  webhookError?: string
}) {
  const [urlCopied, setUrlCopied] = useState(false)
  const [secretCopied, setSecretCopied] = useState(false)

  return (
    <Alert>
      <AlertDescription className="space-y-2">
        <p>
          Git source connected.{' '}
          {webhookError ??
            "This provider doesn't register a webhook automatically yet, add these to the repository's settings for pushes to auto-deploy:"}
        </p>
        <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
          <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
            {webhookUrl}
          </code>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              void navigator.clipboard
                .writeText(webhookUrl)
                .then(() => { setUrlCopied(true) })
            }}
          >
            {urlCopied ? <CheckIcon /> : <CopyIcon />}
            {urlCopied ? 'Copied' : 'Copy'}
          </Button>
        </div>
        <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
          <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
            {webhookSecret}
          </code>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              void navigator.clipboard.writeText(webhookSecret).then(() => {
                setSecretCopied(true)
              })
            }}
          >
            {secretCopied ? <CheckIcon /> : <CopyIcon />}
            {secretCopied ? 'Copied' : 'Copy'}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  )
}
