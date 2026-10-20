import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  TrashIcon,
  GitBranchIcon,
  LockIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useAppBranchEnvOverrides,
  useDeleteAppBranchEnvOverride,
  useSetAppBranchEnvOverride,
} from '../queries/appBranchEnv'

const branchEnvOverrideSchema = z.object({
  branchPattern: z.string().trim().min(1, 'Branch pattern is required'),
  key: z.string().trim().min(1, 'Key is required'),
  value: z.string().min(1, 'Value is required'),
})

type BranchEnvOverrideFormValues = z.infer<typeof branchEnvOverrideSchema>

// Declares (or removes) one env var's value for a specific branch or
// branch pattern (an exact name, or a shell glob like "release/*"),
// applied on top of this app's own env, and on top of any unscoped
// "apps preview-env" override, only when a preview environment is
// created from a matching branch. Listed separately from EnvEditor and
// PreviewEnvOverridesEditor since these two only ever apply to a subset
// of previews, not every preview unconditionally. A secret-marked
// override's value is never returned by the server once saved, the
// same rule SecretsEditor already establishes for per-app secrets.
export function BranchEnvOverridesEditor({ appName }: { appName: string }) {
  const overridesQuery = useAppBranchEnvOverrides(appName)
  const setOverride = useSetAppBranchEnvOverride(appName)
  const deleteOverride = useDeleteAppBranchEnvOverride(appName)
  const [secret, setSecret] = useState(false)
  const { register, handleSubmit, formState, reset } =
    useForm<BranchEnvOverrideFormValues>({
      resolver: zodResolver(branchEnvOverrideSchema),
      defaultValues: { branchPattern: '', key: '', value: '' },
    })

  const onSubmit = handleSubmit((values) => {
    setOverride.mutate(
      {
        branchPattern: values.branchPattern.trim(),
        key: values.key.trim(),
        value: values.value,
        secret,
      },
      {
        onSuccess: () => {
          reset({ branchPattern: '', key: '', value: '' })
          setSecret(false)
          toast.add({ title: 'Branch env override saved.', type: 'success' })
        },
      },
    )
  })

  const overrides = overridesQuery.data ?? []

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitBranchIcon className="size-4" />
          Branch env overrides
        </CardTitle>
        <CardDescription>
          Replaces one env var&apos;s value only when a preview is created from
          a branch matching a pattern below (an exact branch name, or a shell
          glob like <code className="font-mono">release/*</code>), winning over
          both this app&apos;s own env and any unscoped preview env override.
          Never touches this app&apos;s own running deploy, and a non-matching
          branch inherits normally.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {overrides.length > 0 ? (
          <ul className="mb-4 space-y-1 rounded-md border border-border p-2">
            {overrides.map((o) => (
              <li
                key={o.id}
                className="flex items-center justify-between gap-2 rounded px-2 py-1 text-sm"
              >
                <span className="flex min-w-0 flex-wrap items-center gap-2 font-mono">
                  <Badge variant="outline">{o.branchPattern}</Badge>
                  <span>{o.key}</span>
                  {o.secret ? (
                    <span className="flex items-center gap-1 text-muted-foreground">
                      <LockIcon className="size-3.5" />
                      hidden
                    </span>
                  ) : (
                    <span className="text-muted-foreground">{o.value}</span>
                  )}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={deleteOverride.isPending}
                  onClick={() => {
                    deleteOverride.mutate(o.id)
                  }}
                  aria-label={`Remove ${o.key} for ${o.branchPattern}`}
                >
                  <TrashIcon className="size-4 text-muted-foreground" />
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="branch-env-pattern">
                Branch or pattern
              </FieldLabel>
              <Input
                id="branch-env-pattern"
                className="font-mono"
                placeholder="release/*"
                autoComplete="off"
                autoCapitalize="off"
                spellCheck={false}
                {...register('branchPattern')}
              />
              <FieldError
                errors={
                  formState.errors.branchPattern
                    ? [formState.errors.branchPattern]
                    : undefined
                }
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="branch-env-key">Env var</FieldLabel>
              <Input
                id="branch-env-key"
                className="font-mono"
                placeholder="FEATURE_FLAG"
                autoComplete="off"
                autoCapitalize="off"
                spellCheck={false}
                {...register('key')}
              />
              <FieldError
                errors={
                  formState.errors.key ? [formState.errors.key] : undefined
                }
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="branch-env-value">Value</FieldLabel>
              <Input
                id="branch-env-value"
                type={secret ? 'password' : 'text'}
                className="font-mono"
                placeholder="branch-specific value"
                autoComplete="off"
                {...register('value')}
              />
              <FieldError
                errors={
                  formState.errors.value ? [formState.errors.value] : undefined
                }
              />
            </Field>
            <Field orientation="horizontal">
              <Checkbox
                id="branch-env-secret"
                checked={secret}
                onCheckedChange={(checked) => {
                  setSecret(checked === true)
                }}
              />
              <FieldLabel htmlFor="branch-env-secret">
                Store encrypted (secret); never shown again once saved
              </FieldLabel>
            </Field>
          </FieldGroup>
          <div className="mt-3">
            <Button type="submit" size="sm" disabled={setOverride.isPending}>
              {setOverride.isPending ? 'Saving...' : 'Save'}
            </Button>
          </div>
          {setOverride.isError ? (
            <Alert variant="destructive" className="mt-3">
              <AlertDescription>{setOverride.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
