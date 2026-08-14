import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  ClipboardTextIcon,
  PlusIcon,
  BracketsCurlyIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { toast } from '@/components/ui/toast'

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

// Parses a single pasted `.env`-format block into key/value pairs. Scope is
// deliberately limited to what Coolify/Dokploy-style paste-import needs:
// blank lines and `#` comments are skipped, a leading `export ` (as seen in
// .env files copied from shell scripts) is stripped, and a value wrapped in
// matching quotes has them removed. Matching-only quote stripping means a
// stray unmatched quote (e.g. a value that is just `'`) is left as-is rather
// than mangled. Multi-line values and shell variable expansion are out of
// scope on purpose, this is a paste convenience, not a full dotenv parser.
function parseEnvBlock(text: string): { key: string; value: string }[] {
  const results: { key: string; value: string }[] = []
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue

    const withoutExport = line.startsWith('export ')
      ? line.slice('export '.length).trim()
      : line

    const eqIndex = withoutExport.indexOf('=')
    if (eqIndex === -1) continue

    const key = withoutExport.slice(0, eqIndex).trim()
    if (!key) continue

    let value = withoutExport.slice(eqIndex + 1).trim()
    const isDoubleQuoted = value.startsWith('"') && value.endsWith('"')
    const isSingleQuoted = value.startsWith("'") && value.endsWith("'")
    if (value.length >= 2 && (isDoubleQuoted || isSingleQuoted)) {
      value = value.slice(1, -1)
    }

    results.push({ key, value })
  }
  return results
}

// Same add/remove-list, full-replace-PUT pattern as DomainEditor, bound
// to AppDetail.env instead of .domains. Deliberately plain string
// values only: internal/deploy.requireNoUnresolvedEnv (TASKS.md 1.7)
// still rejects app.yaml's `{ secret: true }` / `{ from: ... }` env
// references outright, and store.DesiredService.Env is a flat
// map[string]string with no room to represent a secret reference even
// if this editor tried. The help text below says so explicitly so this
// doesn't read as secret support that just isn't wired up yet.
export function EnvEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const { control, register, handleSubmit, formState, getValues } =
    useForm<EnvFormValues>({
      resolver: zodResolver(envSchema),
      values: { vars: toFieldValues(app.env) },
      resetOptions: { keepDirtyValues: true },
    })
  const { fields, append, remove, update } = useFieldArray({
    control,
    name: 'vars',
  })
  const [pasteOpen, setPasteOpen] = useState(false)
  const [pasteText, setPasteText] = useState('')

  const onSubmit = handleSubmit((values) => {
    const env: Record<string, string> = {}
    for (const v of values.vars) {
      env[v.key.trim()] = v.value
    }
    updateApp.mutate(
      { ...app, env },
      {
        onSuccess: () => {
          toast.add({ title: 'Variables saved.', type: 'success' })
        },
      },
    )
  })

  // Importing a pasted block never submits the form, it only stages rows
  // into the same useFieldArray as manual entry so the existing "Save
  // variables" click and duplicate-key validation are the only path to a
  // real save.
  //
  // A pasted key that matches an existing row (from a prior manual entry or
  // an earlier paste) overwrites that row's value in place instead of being
  // appended as a second row for the duplicate-key validation to flag. This
  // matches how re-pasting an updated .env block should behave: it updates
  // values rather than forcing the operator to delete the stale row first.
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
          Environment variables
        </CardTitle>
        <CardDescription>
          Plain string values only. Secret references are not supported yet, use
          the Secrets card below for values that need to stay encrypted.
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
            <p className="text-sm text-muted-foreground">
              No environment variables set.
            </p>
          ) : (
            <FieldGroup className="gap-2">
              {fields.map((field, index) => (
                <Field key={field.id} orientation="responsive">
                  <div className="flex-1">
                    <Input
                      {...register(`vars.${index}.key`)}
                      className="font-mono"
                      placeholder="KEY"
                    />
                    <FieldError
                      errors={
                        formState.errors.vars?.[index]?.key
                          ? [formState.errors.vars[index]?.key]
                          : undefined
                      }
                    />
                  </div>
                  <div className="flex flex-1 items-start gap-2">
                    <Input
                      {...register(`vars.${index}.value`)}
                      className="flex-1 font-mono"
                      placeholder="value"
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
                  placeholder="DATABASE_URL=postgres://localhost/app"
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
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save variables'}
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
