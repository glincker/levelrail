import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  CheckIcon,
  CopyIcon,
  KeyIcon,
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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { AbilitiesField } from './AbilitiesField'
import { useCreateToken } from '../queries/tokens'
import type { CreateTokenResponse } from '../types/token'

// Expiration options translate directly to expires_in_days on submit.
// 'never' sends no expires_in_days field at all (matches Dokploy's real
// "never" option, finding 10, and createTokenRequest's `omitempty`).
const EXPIRATION_OPTIONS = [
  { value: 'never', label: 'Never', days: undefined },
  { value: '1', label: '1 day', days: 1 },
  { value: '7', label: '7 days', days: 7 },
  { value: '30', label: '30 days', days: 30 },
  { value: '90', label: '90 days', days: 90 },
  { value: '365', label: '1 year', days: 365 },
] as const satisfies {
  value: string
  label: string
  days: number | undefined
}[]

const createTokenSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  expiration: z.enum(['never', '1', '7', '30', '90', '365']),
  abilities: z
    .array(
      z.enum([
        'read',
        'read:sensitive',
        'write',
        'write:sensitive',
        'deploy',
        'root',
      ]),
    )
    .min(1, 'Select at least one ability'),
})

type CreateTokenFormValues = z.infer<typeof createTokenSchema>

export function CreateTokenDialog() {
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState<CreateTokenResponse | null>(null)
  const [copied, setCopied] = useState(false)
  const createToken = useCreateToken()
  const { control, register, handleSubmit, formState, reset } =
    useForm<CreateTokenFormValues>({
      resolver: zodResolver(createTokenSchema),
      defaultValues: { name: '', expiration: 'never', abilities: [] },
    })

  const onSubmit = handleSubmit((values) => {
    const days = EXPIRATION_OPTIONS.find(
      (option) => option.value === values.expiration,
    )?.days
    createToken.mutate(
      {
        name: values.name.trim(),
        abilities: values.abilities,
        ...(days ? { expires_in_days: days } : {}),
      },
      { onSuccess: setCreated },
    )
  })

  // Dismissing the success view is the point of no return: the backend
  // never returns the plaintext again (tokens.go's handleCreateToken doc
  // comment), so closing the dialog is what makes it disappear from view
  // for good, not just a UI reset.
  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setCreated(null)
      setCopied(false)
      reset()
      createToken.reset()
    }
  }

  function copyToken() {
    if (!created) {
      return
    }
    void navigator.clipboard.writeText(created.token).then(() => {
      setCopied(true)
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>Create token</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>Token created</DialogTitle>
              <DialogDescription>
                &ldquo;{created.name}&rdquo; is ready to use.
              </DialogDescription>
            </DialogHeader>
            {/* The one moment this token's plaintext ever exists in the
                UI (tokens.go's handleCreateToken never returns it again),
                so this gets the clearest possible warning treatment, not
                just a description line: Dokploy's copy-once modal (finding
                10) is the model here. No "warning" tone exists in
                badgeVariants/alertVariants, so this reuses the same
                amber-precedent classes AlertRulesPanel.tsx and the deploy
                logs route already established for exactly this gap. */}
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
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={copyToken}
              >
                {copied ? <CheckIcon /> : <CopyIcon />}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>
            <DialogFooter>
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Done
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <KeyIcon className="size-4 text-muted-foreground" />
                Create token
              </DialogTitle>
              <DialogDescription>
                A scoped, revocable credential for the CLI, CI, or an MCP
                integration.
              </DialogDescription>
            </DialogHeader>
            <form
              onSubmit={(e) => {
                void onSubmit(e)
              }}
              className="space-y-4"
            >
              <Field>
                <FieldLabel htmlFor="token-name">Name</FieldLabel>
                <Input
                  id="token-name"
                  placeholder="e.g. ci-deploy"
                  {...register('name')}
                />
                <FieldError errors={[formState.errors.name]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="token-expiration">Expiration</FieldLabel>
                <Controller
                  control={control}
                  name="expiration"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="token-expiration" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {EXPIRATION_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </Field>

              <Controller
                control={control}
                name="abilities"
                render={({ field }) => (
                  <AbilitiesField
                    value={field.value}
                    onChange={field.onChange}
                    error={formState.errors.abilities}
                  />
                )}
              />

              {createToken.isError ? (
                <Alert variant="destructive">
                  <WarningIcon />
                  <AlertDescription>
                    {createToken.error.message}
                  </AlertDescription>
                </Alert>
              ) : null}

              <DialogFooter>
                <Button type="submit" disabled={createToken.isPending}>
                  {createToken.isPending ? 'Creating...' : 'Create token'}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
