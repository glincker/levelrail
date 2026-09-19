import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { TrashIcon, GitBranchIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useClearAppPreviewEnvOverride,
  useSetAppPreviewEnvOverride,
} from '../queries/appPreviewEnv'

const previewEnvOverrideSchema = z.object({
  key: z.string().trim().min(1, 'Key is required'),
  value: z.string().min(1, 'Value is required'),
})

type PreviewEnvOverrideFormValues = z.infer<typeof previewEnvOverrideSchema>

// Declares (or removes) one env var's preview-specific value, applied on
// top of this app's own env only when a preview environment is next
// created from it (a pull request opened or synchronized against a
// preview-enabled, git-connected app). Every other env var not listed
// here still inherits from this app exactly as before. Deliberately not
// built on EnvEditor: this writes through a different endpoint
// (PUT/DELETE .../preview-env/{key}) and never touches this app's own
// running deploy.
export function PreviewEnvOverridesEditor({
  appName,
  overrides,
}: {
  appName: string
  overrides: Record<string, string> | undefined
}) {
  const setOverride = useSetAppPreviewEnvOverride(appName)
  const clearOverride = useClearAppPreviewEnvOverride(appName)
  const { register, handleSubmit, formState, reset } =
    useForm<PreviewEnvOverrideFormValues>({
      resolver: zodResolver(previewEnvOverrideSchema),
      defaultValues: { key: '', value: '' },
    })

  const onSubmit = handleSubmit((values) => {
    setOverride.mutate(
      { key: values.key.trim(), value: values.value },
      {
        onSuccess: () => {
          reset({ key: '', value: '' })
          toast.add({ title: 'Preview env override saved.', type: 'success' })
        },
      },
    )
  })

  const entries = Object.entries(overrides ?? {})

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitBranchIcon className="size-4" />
          Preview env overrides
        </CardTitle>
        <CardDescription>
          Replaces one env var&apos;s value only when a preview environment is
          created for a pull request, never touching this app&apos;s own running
          deploy. Every other env var still inherits normally.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {entries.length > 0 ? (
          <ul className="mb-4 space-y-1 rounded-md border border-border p-2">
            {entries.map(([key, value]) => (
              <li
                key={key}
                className="flex items-center justify-between gap-2 rounded px-2 py-1 text-sm"
              >
                <span className="font-mono">
                  {key}
                  <span className="ml-2 text-muted-foreground">{value}</span>
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={clearOverride.isPending}
                  onClick={() => {
                    clearOverride.mutate(key)
                  }}
                  aria-label={`Remove ${key}`}
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
              <FieldLabel htmlFor="preview-env-key">Env var</FieldLabel>
              <Input
                id="preview-env-key"
                className="font-mono"
                placeholder="SHARED"
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
              <FieldLabel htmlFor="preview-env-value">
                Preview-only value
              </FieldLabel>
              <Input
                id="preview-env-value"
                className="font-mono"
                placeholder="preview-value"
                autoComplete="off"
                {...register('value')}
              />
              <FieldError
                errors={
                  formState.errors.value ? [formState.errors.value] : undefined
                }
              />
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
