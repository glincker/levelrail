import { useState } from 'react'
import {
  ArrowLeftIcon,
  MagnifyingGlassIcon,
  PlugsConnectedIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  useAttachAppIntegration,
  useIntegrationCatalog,
} from '../queries/appIntegrations'
import type { IntegrationCatalogEntry } from '../types/appIntegrations'

function matchesSearch(entry: IntegrationCatalogEntry, query: string): boolean {
  if (!query) return true
  return (
    entry.name.toLowerCase().includes(query) ||
    entry.description.toLowerCase().includes(query) ||
    entry.key.toLowerCase().includes(query)
  )
}

function defaultFieldValues(
  entry: IntegrationCatalogEntry,
): Record<string, string> {
  const defaults: Record<string, string> = {}
  for (const field of entry.env_vars) {
    if (field.default) {
      defaults[field.name] = field.default
    }
  }
  return defaults
}

// Two-step browse-then-configure dialog: search the catalog
// (internal/integrations, GET /api/v1/integrations), pick one, fill in
// its required field(s), attach. Mirrors BrowseTemplatesFields' own
// search-grid shape for a different, per-app-not-per-deploy catalog.
export function AttachIntegrationDialog({
  appName,
  attachedKeys,
}: {
  appName: string
  /** Catalog keys already attached, filtered out of the browse grid. */
  attachedKeys: string[]
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<IntegrationCatalogEntry | null>(null)
  const [fieldValues, setFieldValues] = useState<Record<string, string>>({})
  const [fieldErrors, setFieldErrors] = useState<string[]>([])
  const catalogQuery = useIntegrationCatalog()
  const attach = useAttachAppIntegration(appName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setSearch('')
      setSelected(null)
      setFieldValues({})
      setFieldErrors([])
      attach.reset()
    }
  }

  function handleSelect(entry: IntegrationCatalogEntry) {
    setSelected(entry)
    setFieldValues(defaultFieldValues(entry))
    setFieldErrors([])
  }

  function handleSubmit() {
    if (!selected) return
    const missing = selected.env_vars.filter(
      (field) => field.required && !fieldValues[field.name]?.trim(),
    )
    if (missing.length > 0) {
      setFieldErrors(missing.map((field) => `${field.name} is required`))
      return
    }
    setFieldErrors([])
    attach.mutate(
      { integration_key: selected.key, fields: fieldValues },
      {
        onSuccess: () => {
          toast.add({
            title: 'Integration attached.',
            description: `${selected.name}'s env vars will be injected at ${appName}'s next deploy.`,
            type: 'success',
          })
          handleOpenChange(false)
        },
        onError: (error) => {
          toast.add({
            title: 'Could not attach integration.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  const catalog = (catalogQuery.data ?? []).filter(
    (entry) => !attachedKeys.includes(entry.key),
  )
  const normalizedSearch = search.trim().toLowerCase()
  const filtered = catalog.filter((entry) =>
    matchesSearch(entry, normalizedSearch),
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button type="button" size="sm" />}>
        <PlugsConnectedIcon className="size-3.5" aria-hidden="true" />
        Attach integration
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {selected ? selected.name : 'Attach an integration'}
          </DialogTitle>
          <DialogDescription>
            {selected
              ? 'Fill in the field(s) this integration needs. Values are stored encrypted and never shown again.'
              : 'Env var injection only in this version, no deep webhook or API wiring.'}
          </DialogDescription>
        </DialogHeader>

        {selected ? (
          <div className="space-y-4">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                setSelected(null)
              }}
            >
              <ArrowLeftIcon />
              Back
            </Button>

            {selected.env_vars.map((field) => (
              <Field key={field.name}>
                <FieldLabel htmlFor={`integration-field-${field.name}`}>
                  {field.name}
                  {field.required ? null : ' (optional)'}
                </FieldLabel>
                <Input
                  id={`integration-field-${field.name}`}
                  type={
                    field.type === 'api_key' || field.type === 'token'
                      ? 'password'
                      : 'text'
                  }
                  placeholder={field.placeholder ?? field.default ?? ''}
                  value={fieldValues[field.name] ?? ''}
                  onChange={(e) => {
                    const value = e.target.value
                    setFieldValues((prev) => ({ ...prev, [field.name]: value }))
                  }}
                />
                {field.default ? (
                  <FieldDescription>
                    Defaults to {field.default} if left blank.
                  </FieldDescription>
                ) : null}
              </Field>
            ))}

            <a
              href={selected.docs_url}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-block text-xs text-primary underline underline-offset-4"
            >
              {selected.name} setup docs
            </a>

            {fieldErrors.length > 0 ? (
              <Alert variant="destructive">
                <AlertDescription>{fieldErrors.join(', ')}</AlertDescription>
              </Alert>
            ) : null}
            {attach.isError ? (
              <Alert variant="destructive">
                <AlertDescription>{attach.error.message}</AlertDescription>
              </Alert>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                disabled={attach.isPending}
                onClick={handleSubmit}
              >
                {attach.isPending ? 'Attaching...' : 'Attach'}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="relative">
              <MagnifyingGlassIcon
                className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value)
                }}
                placeholder="Search integrations..."
                aria-label="Search integrations"
                className="pl-8"
              />
            </div>
            {catalogQuery.isLoading ? (
              <div className="grid grid-cols-2 gap-3">
                {Array.from({ length: 4 }).map((_, index) => (
                  <Skeleton key={index} className="h-20 w-full" />
                ))}
              </div>
            ) : catalogQuery.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {catalogQuery.error.message}
                </AlertDescription>
              </Alert>
            ) : (
              <div className="grid grid-cols-2 gap-3">
                {filtered.map((entry) => (
                  <button
                    key={entry.key}
                    type="button"
                    onClick={() => {
                      handleSelect(entry)
                    }}
                    className="flex flex-col items-start gap-1 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:border-primary/40 hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
                  >
                    <span className="text-sm font-medium text-foreground">
                      {entry.name}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {entry.description}
                    </span>
                  </button>
                ))}
              </div>
            )}
            {filtered.length === 0 && !catalogQuery.isLoading ? (
              <p className="py-6 text-center text-sm text-muted-foreground">
                {catalog.length === 0
                  ? 'Every available integration is already attached.'
                  : `No integrations match "${search}".`}
              </p>
            ) : null}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
