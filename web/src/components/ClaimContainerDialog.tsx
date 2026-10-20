import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useClaimOrphanedContainer } from '../queries/containers'

// Mirrors CreateAppFields.tsx's own imageSlugFrom: a client-side preview
// of the name the server will derive if the operator doesn't type one,
// so the dialog doesn't open on a blank field. The server
// (deriveAppNameFromContainer, internal/api/containers_orphaned.go) is
// still the actual source of truth; this is only a starting suggestion.
function slugFromContainerName(containerName: string): string {
  return containerName
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function ClaimContainerDialog({
  containerName,
  open,
  onOpenChange,
}: {
  containerName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [name, setName] = useState(() => slugFromContainerName(containerName))
  const navigate = useNavigate()
  const claim = useClaimOrphanedContainer()

  function handleOpenChange(next: boolean) {
    onOpenChange(next)
    if (!next) {
      claim.reset()
      setName(slugFromContainerName(containerName))
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Claim &ldquo;{containerName}&rdquo;</DialogTitle>
          <DialogDescription>
            Creates a new app from this container&apos;s image. On its first
            deploy, Levelrail replaces this container with a freshly managed
            one; it does not adopt this container&apos;s running data.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="claim-app-name">App name</Label>
          <Input
            id="claim-app-name"
            value={name}
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
        </div>
        {claim.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{claim.error.message}</AlertDescription>
          </Alert>
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
            disabled={claim.isPending || name.trim() === ''}
            onClick={() => {
              claim.mutate(
                { containerName, name: name.trim() },
                {
                  onSuccess: (created) => {
                    onOpenChange(false)
                    toast.add({
                      title: `Claimed "${containerName}" as app "${created.name}".`,
                      type: 'success',
                    })
                    void navigate({
                      to: '/apps/$name',
                      params: { name: created.name },
                    })
                  },
                },
              )
            }}
          >
            {claim.isPending ? 'Claiming...' : 'Claim container'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
