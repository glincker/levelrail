import { useState } from 'react'
import {
  ArrowsLeftRightIcon,
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useOrganizationListOptional,
  useSetProjectOrganization,
} from '../queries/organizations'

// No-organization sentinel, mirrors MoveToProjectDialog's own
// NO_PROJECT_VALUE exactly and for the identical reason: base-ui's
// Select can't use an empty string as an item value, but
// PUT .../organization treats an empty org_id as "no organization".
const NO_ORGANIZATION_VALUE = '__none__'

export function MoveToOrganizationDialog({
  projectId,
  projectName,
  currentOrgId,
}: {
  projectId: string
  projectName: string
  currentOrgId?: string
}) {
  const [open, setOpen] = useState(false)
  const [targetOrgId, setTargetOrgId] = useState(NO_ORGANIZATION_VALUE)
  const setProjectOrganization = useSetProjectOrganization()
  // Optional convenience only, see useOrganizationListOptional's own doc
  // comment: a failure to list organizations must degrade to "no
  // organizations available" rather than crash the project detail page
  // this trigger lives on.
  const organizationList = useOrganizationListOptional()
  const organizations = organizationList.data ?? []

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setTargetOrgId(currentOrgId || NO_ORGANIZATION_VALUE)
    } else {
      setProjectOrganization.reset()
    }
  }

  const resolvedTarget =
    targetOrgId === NO_ORGANIZATION_VALUE ? '' : targetOrgId
  const isNoop = (currentOrgId ?? '') === resolvedTarget

  function handleMove() {
    setProjectOrganization.mutate(
      { projectId, orgId: resolvedTarget },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: `Project "${projectName}" moved.`,
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsLeftRightIcon className="size-3.5" aria-hidden="true" />
        Move
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>
            Move &ldquo;{projectName}&rdquo; to an organization
          </DialogTitle>
          <DialogDescription>
            Purely organizational: this doesn&apos;t change how {projectName}
            &apos;s apps or databases run.
          </DialogDescription>
        </DialogHeader>

        {organizations.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No organizations yet. Create one from Settings first.
          </p>
        ) : (
          <div className="space-y-1.5">
            <label
              htmlFor="move-target-organization"
              className="text-sm font-medium text-foreground"
            >
              Move to
            </label>
            <Select
              value={targetOrgId}
              onValueChange={(value) => {
                if (value) {
                  setTargetOrgId(value)
                }
              }}
            >
              <SelectTrigger id="move-target-organization" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_ORGANIZATION_VALUE}>
                  No organization
                  {currentOrgId ? '' : ' (current)'}
                </SelectItem>
                {organizations.map((o) => (
                  <SelectItem key={o.id} value={o.id}>
                    {o.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {setProjectOrganization.isError ? (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <WarningIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {setProjectOrganization.error.message}
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
            disabled={
              setProjectOrganization.isPending ||
              organizations.length === 0 ||
              isNoop
            }
            onClick={handleMove}
          >
            {setProjectOrganization.isPending ? 'Moving...' : 'Move'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
