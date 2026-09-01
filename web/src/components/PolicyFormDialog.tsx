import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { PencilSimpleIcon, ShieldCheckIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreatePolicy, useUpdatePolicy } from '../queries/iamPolicies'
import type { PolicyResource } from '../queries/iamPolicies'

const EXAMPLE_DOCUMENT = `{
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["read"],
      "Resource": ["app:prod-web"]
    }
  ]
}`

// document is kept as a raw string in the form and only parsed on
// submit: a JSON textarea is easy to type invalid JSON into mid-edit,
// so validating on every keystroke via zod would just flash errors
// while the operator is still typing. Structural checks against the
// Statement/Effect/Action/Resource shape happen in parseDocument below,
// not here: this schema only guards name and non-empty JSON text.
const policyFormSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  description: z.string().trim(),
  documentText: z.string().trim().min(1, 'Document is required'),
})

type PolicyFormValues = z.infer<typeof policyFormSchema>

function defaultValuesFor(policy?: PolicyResource): PolicyFormValues {
  return {
    name: policy?.name ?? '',
    description: policy?.description ?? '',
    documentText: policy ? JSON.stringify(policy.document, null, 2) : EXAMPLE_DOCUMENT,
  }
}

// Mirrors the server's own document shape check (400 on an invalid
// document per the frozen API contract) closely enough to catch typos
// before a round trip, without reimplementing the full policy-engine
// validation: Effect/Action/Resource content is still the server's call.
function parseDocument(text: string): { document?: object; error?: string } {
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    return { error: 'Not valid JSON.' }
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    return { error: 'Document must be a JSON object.' }
  }
  const statement = (parsed as { Statement?: unknown }).Statement
  if (!Array.isArray(statement) || statement.length === 0) {
    return { error: 'Document must have a non-empty "Statement" array.' }
  }
  return { document: parsed }
}

// Handles both create (POST /api/v1/iam/policies) and edit (PUT
// /api/v1/iam/policies/{id}) from the same form: PolicyTable passes an
// existing `policy` to prefill and switch this into edit mode, or
// nothing at all for the "Create policy" trigger on the page header.
export function PolicyFormDialog({
  policy,
  trigger,
}: {
  policy?: PolicyResource
  trigger: React.ReactNode
}) {
  const isEdit = Boolean(policy)
  const [open, setOpen] = useState(false)
  const [documentError, setDocumentError] = useState<string | null>(null)
  const createPolicy = useCreatePolicy()
  const updatePolicy = useUpdatePolicy()
  const mutation = isEdit ? updatePolicy : createPolicy

  const { register, handleSubmit, formState, reset } =
    useForm<PolicyFormValues>({
      resolver: zodResolver(policyFormSchema),
      defaultValues: defaultValuesFor(policy),
    })

  // Reset to the row's current values every time the dialog opens for
  // edit: PolicyTable keeps one PolicyFormDialog mounted per row, so
  // without this a second open would keep showing whatever was typed
  // (and abandoned) the first time it was closed. Done directly in the
  // open-change handler rather than an effect keyed on `open`, so the
  // reset happens exactly once per transition instead of an
  // effect-driven extra render.
  function handleOpenChange(next: boolean) {
    setOpen(next)
    setDocumentError(null)
    if (next) {
      reset(defaultValuesFor(policy))
    } else {
      mutation.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const { document, error } = parseDocument(values.documentText)
    if (error || !document) {
      setDocumentError(error ?? 'Invalid document.')
      return
    }
    setDocumentError(null)

    const req = {
      name: values.name.trim(),
      description: values.description.trim(),
      document: document as PolicyResource['document'],
    }

    if (isEdit && policy) {
      updatePolicy.mutate(
        { id: policy.id, req },
        {
          onSuccess: (updated) => {
            handleOpenChange(false)
            toast.add({
              title: `Policy "${updated.name}" updated.`,
              type: 'success',
            })
          },
        },
      )
    } else {
      createPolicy.mutate(req, {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: `Policy "${created.name}" created.`,
            type: 'success',
          })
        },
      })
    }
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger as React.ReactElement} />
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {isEdit ? (
              <PencilSimpleIcon className="size-4 text-muted-foreground" />
            ) : (
              <ShieldCheckIcon className="size-4 text-muted-foreground" />
            )}
            {isEdit ? `Edit "${policy?.name}"` : 'Create policy'}
          </DialogTitle>
          <DialogDescription>
            A JSON document of Allow/Deny statements, attachable to a user or
            an API token.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="policy-name">Name</FieldLabel>
            <Input
              id="policy-name"
              placeholder="e.g. prod-readers"
              {...register('name')}
            />
            <FieldError errors={[formState.errors.name]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="policy-description">
              Description <span className="text-muted-foreground">(optional)</span>
            </FieldLabel>
            <Input
              id="policy-description"
              placeholder="e.g. read-only on prod apps"
              {...register('description')}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="policy-document">Document</FieldLabel>
            <Textarea
              id="policy-document"
              className="min-h-48 font-mono text-xs"
              spellCheck={false}
              {...register('documentText')}
            />
            <FieldHint>
              A JSON object with a Statement array. Each statement has an
              Effect ("Allow" or "Deny"), an Action list (read,
              read:sensitive, write, write:sensitive, deploy, root, or "*"),
              and a Resource list (e.g. "app:prod-web", "database:main", or
              "*").
            </FieldHint>
            <FieldError errors={[formState.errors.documentText]} />
            {documentError ? (
              <p className="text-sm font-normal text-destructive">
                {documentError}
              </p>
            ) : null}
          </Field>

          {mutation.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{mutation.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending
                ? isEdit
                  ? 'Saving...'
                  : 'Creating...'
                : isEdit
                  ? 'Save changes'
                  : 'Create policy'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
