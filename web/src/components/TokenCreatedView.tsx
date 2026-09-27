import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { CreateTokenResponse } from '../types/token'

// Shows a freshly minted token exactly once. The plaintext is never cached
// (the create mutation invalidates the list instead of storing the
// response), so closing the surrounding dialog is what discards it.
export function TokenCreatedView({
  created,
  onDone,
}: {
  created: CreateTokenResponse
  onDone: () => void
}) {
  const [copied, setCopied] = useState(false)

  function copyToken() {
    void navigator.clipboard.writeText(created.token).then(() => {
      setCopied(true)
    })
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>Token created</DialogTitle>
        <DialogDescription>
          &ldquo;{created.name}&rdquo; is ready to use.
        </DialogDescription>
      </DialogHeader>
      <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
        <WarningIcon className="mt-0.5 size-4 shrink-0" />
        <p className="text-sm">
          Copy this token now. It will not be shown again.
        </p>
      </div>
      <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
        <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
          {created.token}
        </code>
        <Button type="button" size="sm" variant="outline" onClick={copyToken}>
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
      <DialogFooter>
        <Button type="button" onClick={onDone}>
          Done
        </Button>
      </DialogFooter>
    </>
  )
}
