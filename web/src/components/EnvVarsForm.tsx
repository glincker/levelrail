import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useFieldArray, useForm, useWatch, type Control } from 'react-hook-form'
import { z } from 'zod'
import {
  ClipboardTextIcon,
  PlusIcon,
  BracketsCurlyIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { parseEnvBlock } from '../lib/envParse'
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldError, FieldGroup } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

const envSchema = z
  .object({
    vars: z.array(
      z.object({
        key: z.string().trim().min(1, 'Key is required'),
        value: z.string(),
      }),
    ),
  })
  .superRefine((data, ctx) => {
    const seen = new Set<string>()
    data.vars.forEach((v, index) => {
      const key = v.key.trim()
      if (key && seen.has(key)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Duplicate key',
          path: ['vars', index, 'key'],
        })
      }
      seen.add(key)
    })
  })

type EnvFormValues = z.infer<typeof envSchema>

function toFieldValues(
  env?: Record<string, string>,
): { key: string; value: string }[] {
  return Object.entries(env ?? {}).map(([key, value]) => ({ key, value }))
}

export interface EnvVarsFormProps {
  title: string
  description: React.ReactNode
  emptyMessage: string
  pastePlaceholder: string
  values?: Record<string, string>
  isPending: boolean
  errorMessage?: string
  onSave: (vars: Record<string, string>) => void
  // Renders next to a row's key input, keyed off that row's live (not
  // just initial) key value: EnvEditor uses this to show which shared
  // tier, if any, an effective value would fall back to.
  renderKeyBadge?: (key: string) => React.ReactNode
  // Read-only rows for keys the effective env resolves from a shared
  // tier that this editor's own values map doesn't set, so they're
  // visible without being part of the editable/save-able field array.
  inheritedRows?: { key: string; value: string; badge: React.ReactNode }[]
}

// Wraps a single row's key badge in its own component so only that
// row re-renders on keystroke, via useWatch scoped to one field path.
function EnvRowBadge({
  control,
  index,
  renderKeyBadge,
}: {
  control: Control<EnvFormValues>
  index: number
  renderKeyBadge: (key: string) => React.ReactNode
}) {
  const key = useWatch({ control, name: `vars.${index}.key` })
  return <>{renderKeyBadge(key)}</>
}

// Shared add/remove-list, paste-.env-import, full-replace-save form,
// used by both EnvEditor (an app's own env) and OrganizationEnvEditor
// (an organization's shared env): identical interaction shape, only the
// data source, copy, and save mutation differ per caller. renderKeyBadge
// and inheritedRows are optional and only used by EnvEditor, to show
// shared-tier provenance inline.
export function EnvVarsForm({
  title,
  description,
  emptyMessage,
  pastePlaceholder,
  values,
  isPending,
  errorMessage,
  onSave,
  renderKeyBadge,
  inheritedRows,
}: EnvVarsFormProps) {
  const { control, register, handleSubmit, formState, getValues } =
    useForm<EnvFormValues>({
      resolver: zodResolver(envSchema),
      values: { vars: toFieldValues(values) },
      resetOptions: { keepDirtyValues: true },
    })
  const { fields, append, remove, update } = useFieldArray({
    control,
    name: 'vars',
  })
  const [pasteOpen, setPasteOpen] = useState(false)
  const [pasteText, setPasteText] = useState('')

  const onSubmit = handleSubmit((formValues) => {
    const vars: Record<string, string> = {}
    for (const v of formValues.vars) {
      vars[v.key.trim()] = v.value
    }
    onSave(vars)
  })

  // Importing a pasted block never submits the form, it only stages rows
  // into the same useFieldArray as manual entry so the existing "Save
  // variables" click and duplicate-key validation are the only path to a
  // real save.
  //
  // A pasted key that matches an existing row (from a prior manual entry
  // or an earlier paste) overwrites that row's value in place instead of
  // being appended as a second row for the duplicate-key validation to
  // flag, matching how re-pasting an updated .env block should behave.
  // The same last-one-wins rule also resolves duplicate keys within the
  // pasted block itself, matching real .env semantics.
  const handleImportPaste = () => {
    const parsed = parseEnvBlock(pasteText)
    if (parsed.length > 0) {
      const currentVars = getValues('vars')
      for (const entry of parsed) {
        const existingIndex = currentVars.findIndex((v) => v.key === entry.key)
        if (existingIndex === -1) {
          currentVars.push(entry)
          append(entry)
        } else {
          currentVars[existingIndex] = entry
          update(existingIndex, entry)
        }
      }
    }
    setPasteText('')
    setPasteOpen(false)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BracketsCurlyIcon className="size-4" />
          {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-3"
        >
          {fields.length === 0 ? (
            <p className="text-sm text-muted-foreground">{emptyMessage}</p>
          ) : (
            <FieldGroup className="gap-2">
              {fields.map((field, index) => (
                <Field key={field.id} orientation="responsive">
                  <div className="flex-1">
                    <Input
                      {...register(`vars.${index}.key`)}
                      className="font-mono"
                      placeholder="KEY"
                      aria-label="Variable name"
                    />
                    <FieldError
                      errors={
                        formState.errors.vars?.[index]?.key
                          ? [formState.errors.vars[index]?.key]
                          : undefined
                      }
                    />
                    {renderKeyBadge ? (
                      <div className="mt-1">
                        <EnvRowBadge
                          control={control}
                          index={index}
                          renderKeyBadge={renderKeyBadge}
                        />
                      </div>
                    ) : null}
                  </div>
                  <div className="flex flex-1 items-start gap-2">
                    <Input
                      {...register(`vars.${index}.value`)}
                      className="flex-1 font-mono"
                      placeholder="value"
                      aria-label="Variable value"
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
                      <span className="sr-only">Remove variable</span>
                    </Button>
                  </div>
                </Field>
              ))}
            </FieldGroup>
          )}
          {inheritedRows && inheritedRows.length > 0 ? (
            <div className="space-y-2 rounded-md border border-dashed border-border p-3">
              <p className="text-xs font-medium text-muted-foreground">
                Inherited from a shared tier, not part of this app&rsquo;s own
                variables
              </p>
              <div className="space-y-1.5">
                {inheritedRows.map((row) => (
                  <div
                    key={row.key}
                    className="flex flex-wrap items-center gap-2 text-sm"
                  >
                    <span className="font-mono text-foreground">
                      {row.key}
                    </span>
                    <span className="max-w-[16rem] truncate font-mono text-muted-foreground">
                      {row.value}
                    </span>
                    {row.badge}
                  </div>
                ))}
              </div>
            </div>
          ) : null}
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
              Add variable
            </Button>
            <Dialog open={pasteOpen} onOpenChange={setPasteOpen}>
              <DialogTrigger
                render={<Button type="button" variant="outline" size="sm" />}
              >
                <ClipboardTextIcon />
                Paste .env
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Paste .env</DialogTitle>
                  <DialogDescription>
                    Paste raw .env-format text. Comments and blank lines are
                    skipped, and quoted values are unwrapped. This only stages
                    rows, click Save variables afterward to apply them.
                  </DialogDescription>
                </DialogHeader>
                <Textarea
                  value={pasteText}
                  onChange={(e) => {
                    setPasteText(e.target.value)
                  }}
                  className="min-h-40 font-mono"
                  placeholder={pastePlaceholder}
                  autoFocus
                />
                <DialogFooter>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      setPasteText('')
                      setPasteOpen(false)
                    }}
                  >
                    Cancel
                  </Button>
                  <Button
                    type="button"
                    onClick={handleImportPaste}
                    disabled={pasteText.trim().length === 0}
                  >
                    Import
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
            <Button type="submit" size="sm" disabled={isPending}>
              {isPending ? 'Saving...' : 'Save variables'}
            </Button>
          </div>
          {errorMessage ? (
            <Alert variant="destructive">
              <AlertDescription>{errorMessage}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
