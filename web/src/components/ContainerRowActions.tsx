import { useState } from 'react'
import {
  DotsThreeIcon,
  PauseIcon,
  TagIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { toast } from '@/components/ui/toast'
import { useStopOrphanedContainer } from '../queries/containers'
import { ClaimContainerDialog } from './ClaimContainerDialog'
import { RemoveContainerDialog } from './RemoveContainerDialog'

// Row actions for an orphaned (Managed: false) container only: Stop is a
// direct action (reversible, an operator can start it again by hand),
// Remove and Claim each open their own confirm/edit dialog since both
// are one-way (containers.tsx never renders this for a Managed row).
export function ContainerRowActions({ name }: { name: string }) {
  const [confirmRemove, setConfirmRemove] = useState(false)
  const [claiming, setClaiming] = useState(false)
  const stopContainer = useStopOrphanedContainer()

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={`Actions for ${name}`}
            />
          }
        >
          <DotsThreeIcon weight="bold" />
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem
            disabled={stopContainer.isPending}
            onClick={() => {
              stopContainer.mutate(name, {
                onSuccess: () => {
                  toast.add({ title: `Stopped "${name}".`, type: 'success' })
                },
                onError: (error) => {
                  toast.add({ title: error.message, type: 'error' })
                },
              })
            }}
          >
            <PauseIcon />
            Stop
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => {
              setClaiming(true)
            }}
          >
            <TagIcon />
            Claim as app
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => {
              setConfirmRemove(true)
            }}
          >
            <TrashIcon />
            Remove
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <RemoveContainerDialog
        containerName={name}
        open={confirmRemove}
        onOpenChange={setConfirmRemove}
      />
      <ClaimContainerDialog
        containerName={name}
        open={claiming}
        onOpenChange={setClaiming}
      />
    </>
  )
}
