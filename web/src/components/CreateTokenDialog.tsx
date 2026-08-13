import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { CheckIcon, CopyIcon } from 'lucide-react'
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
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { useCreateToken } from '../queries/tokens'
import type { Ability, CreateTokenResponse } from '../types/token'

// Ability picker copy, one line each, per Coolify's checkbox model
// (docs-local/research/competitor-onboarding-auth-ux.md finding 9): a
// small, legible permission surface, not a permission matrix.
const ABILITY_OPTIONS: { value: Ability; label: string; hint: string }[] = [
  {
    value: 'read',
    label: 'Read',
    hint: 'List and view apps, deploys, domains.',
  },
  {
    value: 'read:sensitive',
    label: 'Read sensitive',
    hint: 'Read env vars and other sensitive fields.',
  },
  {
    value: 'write',
    label: 'Write',
    hint: 'Edit app config, domains, env vars.',
  },
  { value: 'deploy', label: 'Deploy', hint: 'Trigger deploys and rollbacks.' },
  {
    value: 'root',
    label: 'Root',
    hint: 'Everything below. Exclusive of every other ability.',
  },
]

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
    .array(z.enum(['read', 'read:sensitive', 'write', 'deploy', 'root']))
    .min(1, 'Select at least one ability'),
})

type CreateTokenFormValues = z.infer<typeof createTokenSchema>

// Coolify's own exclusivity rule (finding 9 / abilities.go's
// validateAbilities): selecting root clears every other checkbox;
// selecting any other checkbox clears root if it was set. Root is never
// left combined with anything else, matching what the server would
// reject anyway, rather than letting the UI produce an invalid
// combination for validateAbilities to bounce back as a 400.
function toggleAbility(current: Ability[], ability: Ability): Ability[] {
  if (ability === 'root') {
    return current.includes('root') ? [] : ['root']
  }
  const withoutRoot = current.filter((a) => a !== 'root')
  return withoutRoot.includes(ability)
    ? withoutRoot.filter((a) => a !== ability)
    : [...withoutRoot, ability]
}

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
                Copy this token now. You will not be able to see it again.
              </DialogDescription>
            </DialogHeader>
            <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
              <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                {created.token}
              </code>
              <Button
                type="button"
                size="icon-sm"
                variant="outline"
                onClick={copyToken}
              >
                {copied ? <CheckIcon /> : <CopyIcon />}
                <span className="sr-only">Copy token</span>
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
              <DialogTitle>Create token</DialogTitle>
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
                <FieldLabel>Abilities</FieldLabel>
                <Controller
                  control={control}
                  name="abilities"
                  render={({ field }) => (
                    <div className="space-y-2">
                      {ABILITY_OPTIONS.map((option) => {
                        const rootSelected = field.value.includes('root')
                        const disabled = rootSelected && option.value !== 'root'
                        return (
                          <label
                            key={option.value}
                            className="flex items-start gap-2 text-sm"
                          >
                            <Checkbox
                              className="mt-0.5"
                              checked={field.value.includes(option.value)}
                              disabled={disabled}
                              onCheckedChange={() => {
                                field.onChange(
                                  toggleAbility(field.value, option.value),
                                )
                              }}
                            />
                            <span>
                              <span className="font-medium text-foreground">
                                {option.label}
                              </span>
                              <span className="block text-xs text-muted-foreground">
                                {option.hint}
                              </span>
                            </span>
                          </label>
                        )
                      })}
                    </div>
                  )}
                />
                <FieldError errors={[formState.errors.abilities]} />
              </Field>

              {createToken.isError ? (
                <p className="text-sm text-destructive">
                  {createToken.error.message}
                </p>
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
