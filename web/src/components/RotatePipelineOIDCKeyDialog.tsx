import { useState } from 'react'
import {
  ArrowsClockwiseIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRotatePipelineOIDCKey } from '../queries/pipelineOidc'

// Confirm-dialog for POST /api/v1/pipelines/oidc/rotate-key, mirroring
// RotateMeshKeyDialog's shape: the previous signing key is not removed
// on rotation, it stays published in the JWKS until its grace period
// elapses, so a token minted moments before rotation keeps verifying.
// The warning below exists because that propagation window is easy to
// get wrong: an operator who assumes "rotated" means "the old key is
// gone" could otherwise revoke it elsewhere too early.
export function RotatePipelineOIDCKeyDialog() {
  const [open, setOpen] = useState(false)
  const rotate = useRotatePipelineOIDCKey()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      rotate.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsClockwiseIcon className="size-3.5" aria-hidden="true" />
        Rotate signing key
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <WarningIcon className="size-4 text-amber-600" aria-hidden="true" />
            Rotate the pipeline OIDC signing key?
          </DialogTitle>
          <DialogDescription>
            Generates a fresh signing key and uses it for every new token from
            now on. The previous key is not removed: it stays published in the
            JWKS for 24 hours so tokens it already signed, and any cloud
            provider still caching the old JWKS document, keep verifying. Only
            once that window passes does the old key actually disappear from the
            published set.
          </DialogDescription>
        </DialogHeader>
        {rotate.isError ? (
          <p className="text-sm text-destructive">{rotate.error.message}</p>
        ) : null}
        {rotate.isSuccess ? (
          <p className="text-sm text-muted-foreground">
            New key {rotate.data.new_kid}. Previous key {rotate.data.old_kid}{' '}
            retires at {new Date(rotate.data.retire_at).toLocaleString()}.
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={rotate.isPending}
            onClick={() => {
              rotate.mutate(undefined, {
                onSuccess: (result) => {
                  toast.add({
                    title: 'Pipeline OIDC signing key rotated.',
                    description: `Previous key stays published until ${new Date(
                      result.retire_at,
                    ).toLocaleString()}.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {rotate.isPending ? 'Rotating...' : 'Rotate key'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
