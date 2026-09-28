import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { KeyIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { TokenCreatedView } from './TokenCreatedView'
import { InfoTip } from './kit'
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
  agentName: z.string().trim().max(64, 'At most 64 characters'),
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
  const createToken = useCreateToken()
  const { control, register, handleSubmit, formState, reset } =
    useForm<CreateTokenFormValues>({
      resolver: zodResolver(createTokenSchema),
      defaultValues: {
        name: '',
        expiration: 'never',
        agentName: '',
        abilities: [],
      },
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
        ...(values.agentName.trim()
          ? { agent: { name: values.agentName.trim() } }
          : {}),
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
      reset()
      createToken.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>Create token</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {created ? (
          <TokenCreatedView
            created={created}
            onDone={() => {
              handleOpenChange(false)
            }}
          />
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

              <Field>
                <div className="flex items-center gap-1">
                  <FieldLabel htmlFor="token-agent">
                    Agent name (optional)
                  </FieldLabel>
                  <InfoTip label="About agent names">
                    Label the token as issued to an AI agent, for example
                    &ldquo;Claude Code&rdquo;. The name is shown here and on
                    every audit log entry the token makes, so you can tell agent
                    changes from human ones.
                  </InfoTip>
                </div>
                <Input
                  id="token-agent"
                  placeholder="e.g. Claude Code"
                  {...register('agentName')}
                />
                <FieldError errors={[formState.errors.agentName]} />
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
