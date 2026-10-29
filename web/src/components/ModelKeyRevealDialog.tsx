import { CopyGlassIcon, KeyGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { toast } from '@/components/ui/toast'
import { curlExample } from '../lib/modelKeyForm'

// Shows a model's API key exactly once. The server stores only a hash, so
// closing this dialog is the last chance to copy it.
export function ModelKeyRevealDialog({
  modelName,
  apiKey,
  baseUrl,
  onClose,
}: {
  modelName: string
  apiKey: string
  baseUrl?: string
  onClose: () => void
}) {
  function copy(value: string, label: string) {
    void navigator.clipboard.writeText(value).then(() => {
      toast.add({ title: `${label} copied.`, type: 'success' })
    })
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <KeyGlassIcon className="size-5" aria-hidden="true" />
            API key for &ldquo;{modelName}&rdquo;
          </DialogTitle>
          <DialogDescription>
            Copy this key now. It is not shown again; you can rotate it later to
            get a new one.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="flex items-center gap-2 rounded-md border border-border bg-muted p-2">
            <code
              className="min-w-0 flex-1 break-all font-mono text-xs"
              data-testid="model-api-key"
            >
              {apiKey}
            </code>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                copy(apiKey, 'API key')
              }}
            >
              <CopyGlassIcon size="1.4em" aria-hidden="true" />
              Copy
            </Button>
          </div>
          {baseUrl ? (
            <div className="space-y-1">
              <span className="text-xs text-muted-foreground">Try it</span>
              <CodeBlock
                code={curlExample(baseUrl, apiKey)}
                language="bash"
                secret
              />
            </div>
          ) : null}
          {baseUrl ? (
            <p className="text-xs text-muted-foreground">
              OpenAI-compatible base URL:{' '}
              <button
                type="button"
                className="font-mono underline"
                onClick={() => {
                  copy(baseUrl, 'Base URL')
                }}
              >
                {baseUrl}
              </button>
            </p>
          ) : (
            <p className="text-xs text-muted-foreground">
              This model has no public hostname yet. Set a domain when
              deploying, or set APP_PUBLIC_HOST to a public IP for a zero-config
              one.
            </p>
          )}
        </div>
        <DialogFooter>
          <Button type="button" onClick={onClose}>
            I have saved the key
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
