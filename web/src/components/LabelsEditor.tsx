import { zodResolver } from '@hookform/resolvers/zod'
import { useFieldArray, useForm } from 'react-hook-form'
import { z } from 'zod'
import { PlusIcon, TagIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldError, FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'

// Mirrors internal/spec.ReservedLabelPrefix (internal/spec/labels.go)
// exactly: the platform's own reserved Docker label namespace, kept out
// of operator hands so it stays open for this platform's own bookkeeping
// labels later. Not the product name (CLAUDE.md's "no product name
// string in source" rule doesn't apply here), so it's fine as a plain
// literal on this side too; internal/spec.ValidateLabels is still the
// real enforcement, this is just an early, friendlier error before a
// round trip to the server.
const RESERVED_LABEL_PREFIX = 'platform-reserved.'

const labelsSchema = z
  .object({
    labels: z.array(
      z.object({
        key: z.string().trim().min(1, 'Key is required'),
        value: z.string(),
      }),
    ),
  })
  .superRefine((data, ctx) => {
    const seen = new Set<string>()
    data.labels.forEach((l, index) => {
      const key = l.key.trim()
      if (key && seen.has(key)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Duplicate key',
          path: ['labels', index, 'key'],
        })
      }
      seen.add(key)
      if (key.startsWith(RESERVED_LABEL_PREFIX)) {
        ctx.addIssue({
          code: 'custom',
          message: `Reserved for the platform (${RESERVED_LABEL_PREFIX}*)`,
          path: ['labels', index, 'key'],
        })
      }
    })
  })

type LabelsFormValues = z.infer<typeof labelsSchema>

function toFieldValues(
  labels?: Record<string, string>,
): { key: string; value: string }[] {
  return Object.entries(labels ?? {}).map(([key, value]) => ({ key, value }))
}

// Same add/remove-list, full-replace-PUT pattern EnvEditor/
// ResourceLimitsEditor already establish, bound to AppDetail.labels
// instead of .env. A deliberately separate component from EnvEditor
// rather than a generalized "key-value editor" both share: EnvEditor is
// under active work elsewhere for an unrelated unified-env-var-UI
// feature, and touching it here for a shared abstraction would risk a
// merge collision for no real gain, this editor is small enough that a
// little duplicated row-editor boilerplate is the safer trade.
export function LabelsEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const { control, register, handleSubmit, formState } =
    useForm<LabelsFormValues>({
      resolver: zodResolver(labelsSchema),
      values: { labels: toFieldValues(app.labels) },
      resetOptions: { keepDirtyValues: true },
    })
  const { fields, append, remove } = useFieldArray({
    control,
    name: 'labels',
  })

  const onSubmit = handleSubmit((values) => {
    const labels: Record<string, string> = {}
    for (const l of values.labels) {
      labels[l.key.trim()] = l.value
    }
    updateApp.mutate(
      { ...app, labels },
      {
        onSuccess: () => {
          toast.add({ title: 'Labels saved.', type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <TagIcon className="size-4" />
          Custom labels
        </CardTitle>
        <CardDescription>
          Arbitrary Docker labels applied to the container at create time, for
          external tooling that keys off container labels (monitoring agents, log
          shippers, homegrown scripts). Keys starting with{' '}
          <code className="font-mono">{RESERVED_LABEL_PREFIX}</code> are reserved
          for the platform.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-3"
        >
          {fields.length === 0 ? (
            <p className="text-sm text-muted-foreground">No custom labels set.</p>
          ) : (
            <FieldGroup className="gap-2">
              {fields.map((field, index) => (
                <Field key={field.id} orientation="responsive">
                  <div className="flex-1">
                    <Input
                      {...register(`labels.${index}.key`)}
                      className="font-mono"
                      placeholder="key"
                      aria-label="Label key"
                    />
                    <FieldError
                      errors={
                        formState.errors.labels?.[index]?.key
                          ? [formState.errors.labels[index]?.key]
                          : undefined
                      }
                    />
                  </div>
                  <div className="flex flex-1 items-start gap-2">
                    <Input
                      {...register(`labels.${index}.value`)}
                      className="flex-1 font-mono"
                      placeholder="value"
                      aria-label="Label value"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => {
                        remove(index)
                      }}
                    >
                      <XIcon />
                      <span className="sr-only">Remove label</span>
                    </Button>
                  </div>
                </Field>
              ))}
            </FieldGroup>
          )}
          <div className="flex items-center gap-2 pt-1">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                append({ key: '', value: '' })
              }}
            >
              <PlusIcon />
              Add label
            </Button>
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save labels'}
            </Button>
          </div>
          {updateApp.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateApp.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
