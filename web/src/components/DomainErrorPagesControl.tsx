import { useState } from 'react'
import { FileHtmlIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import {
  DOMAIN_ERROR_PAGE_STATUS_CODES,
  useClearDomainErrorPage,
  useDomainErrorPages,
  useSetDomainErrorPage,
} from '../queries/domainErrorPages'

// Replace Caddy's bare default error text (or whatever the backend
// itself returned) with an operator's own HTML for one of a fixed set
// of status codes, enforced by the embedded Caddy ingress
// (internal/reconcile/ingress) on the next reconcile pass. Collapsed by
// default the same way DomainRedirectControl/DomainWafControl are: a
// status badge plus a toggle, expanding into the list of currently
// configured pages and a form to add another.
export function DomainErrorPagesControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { data, isLoading } = useDomainErrorPages(appName, domain)
  const [open, setOpen] = useState(false)
  const [draftCode, setDraftCode] = useState<string | null>(null)
  const [draftBody, setDraftBody] = useState('')
  const setPage = useSetDomainErrorPage(appName, domain)
  const clearPage = useClearDomainErrorPage(appName, domain)

  if (isLoading) {
    return (
      <div
        className="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
        aria-hidden="true"
      >
        <Skeleton className="h-5 w-32 rounded-full" />
        <Skeleton className="h-7 w-32" />
      </div>
    )
  }

  const pages = data?.pages ?? []
  const configuredCodes = new Set(pages.map((p) => p.status_code))
  const availableCodes = DOMAIN_ERROR_PAGE_STATUS_CODES.filter(
    (code) => !configuredCodes.has(code),
  )
  const active = pages.length > 0

  function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    if (!draftCode) return
    setPage.mutate(
      { status_code: Number.parseInt(draftCode, 10), body: draftBody },
      {
        onSuccess: () => {
          setDraftCode(null)
          setDraftBody('')
        },
      },
    )
  }

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-1.5 text-left"
        >
          {active ? (
            <Badge variant="success" className="shrink-0">
              <FileHtmlIcon className="size-3" />
              {pages.length} custom error page{pages.length === 1 ? '' : 's'}
            </Badge>
          ) : (
            <Badge variant="muted" className="shrink-0">
              <FileHtmlIcon className="size-3" />
              No custom error pages
            </Badge>
          )}
        </button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setOpen((v) => !v)}
        >
          <FileHtmlIcon className="size-3.5" />
          {open ? 'Hide' : active ? 'Manage' : 'Add error page'}
        </Button>
      </div>

      {open ? (
        <div className="mt-3 space-y-3">
          {pages.map((page) => (
            <div
              key={page.status_code}
              className="flex items-center justify-between gap-2 rounded border border-border bg-background px-2.5 py-1.5"
            >
              <span className="font-mono text-xs">
                {page.status_code} · {page.body.length} bytes
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={clearPage.isPending}
                onClick={() => clearPage.mutate(page.status_code)}
              >
                Remove
              </Button>
            </div>
          ))}

          {availableCodes.length > 0 ? (
            <form onSubmit={handleAdd} className="space-y-3">
              <Field>
                <FieldLabel htmlFor={`error-page-code-${domain}`}>
                  Status code
                </FieldLabel>
                <Select
                  value={draftCode ?? undefined}
                  onValueChange={(value) => setDraftCode(value)}
                  disabled={setPage.isPending}
                >
                  <SelectTrigger id={`error-page-code-${domain}`}>
                    <SelectValue placeholder="Choose a status code" />
                  </SelectTrigger>
                  <SelectContent>
                    {availableCodes.map((code) => (
                      <SelectItem key={code} value={String(code)}>
                        {code}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel htmlFor={`error-page-body-${domain}`}>
                  Custom HTML
                </FieldLabel>
                <Textarea
                  id={`error-page-body-${domain}`}
                  value={draftBody}
                  onChange={(e) => setDraftBody(e.target.value)}
                  disabled={setPage.isPending}
                  placeholder="<html>...</html>"
                  rows={5}
                />
                <FieldDescription>
                  Served with the same status code, replacing whatever the
                  backend (or Caddy itself, if the container is unreachable)
                  would otherwise return.
                </FieldDescription>
              </Field>

              {setPage.isError ? (
                <Alert variant="destructive">
                  <WarningIcon />
                  <AlertDescription>{setPage.error.message}</AlertDescription>
                </Alert>
              ) : null}
              {clearPage.isError ? (
                <Alert variant="destructive">
                  <WarningIcon />
                  <AlertDescription>{clearPage.error.message}</AlertDescription>
                </Alert>
              ) : null}

              <Button
                type="submit"
                size="sm"
                disabled={
                  setPage.isPending || !draftCode || draftBody.length === 0
                }
              >
                {setPage.isPending ? 'Saving...' : 'Save'}
              </Button>
            </form>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
