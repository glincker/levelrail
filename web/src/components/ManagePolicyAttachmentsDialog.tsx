import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { LinkIcon, TrashIcon, UsersIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import {
  policyAttachmentsQueryOptions,
  useAttachPrincipal,
  useDetachPrincipal,
} from '../queries/iamPolicies'
import type { PolicyResource, PrincipalType } from '../queries/iamPolicies'

const PRINCIPAL_TYPE_LABEL: Record<PrincipalType, string> = {
  user: 'User',
  token: 'Token',
}

// Attach/detach principals for one policy. GET/POST
// /api/v1/iam/policies/{id}/attachments is fetched with a plain useQuery
// here, not the route's useSuspenseQuery: this list only loads once the
// dialog opens (enabled: open), so there is no server-rendered/prefetched
// data to suspend on, matching GitLabAppProjectsCard.tsx's own
// isLoading-driven dialog content instead.
export function ManagePolicyAttachmentsDialog({
  policy,
  trigger,
}: {
  policy: PolicyResource
  trigger: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [principalType, setPrincipalType] = useState<PrincipalType>('user')
  const [principalId, setPrincipalId] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  const attachments = useQuery({
    ...policyAttachmentsQueryOptions(policy.id),
    enabled: open,
  })
  const attach = useAttachPrincipal()
  const detach = useDetachPrincipal()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setPrincipalId('')
      setFormError(null)
      attach.reset()
      detach.reset()
    }
  }

  function handleAttach() {
    const trimmed = principalId.trim()
    if (!trimmed) {
      setFormError('Principal ID is required.')
      return
    }
    setFormError(null)
    attach.mutate(
      { policyId: policy.id, principal_type: principalType, principal_id: trimmed },
      {
        onSuccess: () => {
          setPrincipalId('')
          toast.add({
            title: `Attached ${PRINCIPAL_TYPE_LABEL[principalType].toLowerCase()} "${trimmed}".`,
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger as React.ReactElement} />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <UsersIcon className="size-4 text-muted-foreground" />
            Attachments for &ldquo;{policy.name}&rdquo;
          </DialogTitle>
          <DialogDescription>
            Users and tokens this policy is attached to.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Field orientation="responsive">
            <FieldLabel htmlFor="attach-principal-type">Type</FieldLabel>
            <Select
              value={principalType}
              onValueChange={(value) => {
                setPrincipalType(value as PrincipalType)
              }}
            >
              <SelectTrigger id="attach-principal-type" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="user">User</SelectItem>
                <SelectItem value="token">Token</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="attach-principal-id">Principal ID</FieldLabel>
            <div className="flex gap-2">
              <Input
                id="attach-principal-id"
                placeholder={principalType === 'user' ? 'usr_...' : 'tok_...'}
                value={principalId}
                onChange={(e) => {
                  setPrincipalId(e.target.value)
                }}
              />
              <Button
                type="button"
                onClick={handleAttach}
                disabled={attach.isPending}
              >
                {attach.isPending ? 'Attaching...' : 'Attach'}
              </Button>
            </div>
            <FieldError errors={[formError ? { message: formError } : undefined]} />
          </Field>
          {attach.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{attach.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </div>

        <div className="space-y-2">
          {attachments.isLoading ? (
            <p className="text-sm text-muted-foreground">Loading attachments...</p>
          ) : attachments.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{attachments.error.message}</AlertDescription>
            </Alert>
          ) : (attachments.data ?? []).length === 0 ? (
            <EmptyState
              icon={<LinkIcon className="size-5" />}
              title="No attachments yet"
              description="Attach a user or token above to grant it this policy's Allow/Deny rules."
            />
          ) : (
            <ul className="divide-y divide-border rounded-lg border border-border">
              {(attachments.data ?? []).map((attachment) => {
                const isDetaching =
                  detach.isPending &&
                  detach.variables?.principal_type === attachment.principal_type &&
                  detach.variables?.principal_id === attachment.principal_id
                return (
                  <li
                    key={attachment.id}
                    className="flex items-center justify-between gap-2 px-3 py-2"
                  >
                    <div className="flex items-center gap-2">
                      <Badge variant="outline">
                        {PRINCIPAL_TYPE_LABEL[attachment.principal_type]}
                      </Badge>
                      <span className="font-mono text-sm text-foreground">
                        {attachment.principal_id}
                      </span>
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      disabled={isDetaching}
                      aria-label={`Detach ${attachment.principal_id}`}
                      onClick={() => {
                        detach.mutate(
                          {
                            policyId: policy.id,
                            principal_type: attachment.principal_type,
                            principal_id: attachment.principal_id,
                          },
                          {
                            onSuccess: () => {
                              toast.add({
                                title: `Detached "${attachment.principal_id}".`,
                                type: 'success',
                              })
                            },
                          },
                        )
                      }}
                    >
                      <TrashIcon className="text-destructive" />
                    </Button>
                  </li>
                )
              })}
            </ul>
          )}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
