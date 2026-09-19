import { useState } from 'react'
import { SignpostIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useClearDomainRedirect,
  useDomainRedirect,
  useSetDomainRedirect,
  type SetDomainRedirectRequest,
} from '../queries/domainRedirect'

// Redirect one already-saved domain to a target URL, enforced by the
// embedded Caddy ingress on the next reconcile pass
// (internal/reconcile/ingress). Collapsed by default the same way
// DomainWafControl is: a status badge plus a toggle, expanding into a
// form for the target URL and the permanent/temporary choice. If the
// same domain also has maintenance mode enabled, maintenance mode takes
// precedence server-side and this redirect does not apply; this control
// does not itself warn about that combination, since DomainMaintenanceControl
// already sits right above it in DomainEditor and makes that state visible.
export function DomainRedirectControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { data: redirect, isLoading } = useDomainRedirect(appName, domain)
  const [open, setOpen] = useState(false)
  const [targetURL, setTargetURL] = useState<string | null>(null)
  const [statusCode, setStatusCode] = useState<string | null>(null)
  const setRedirect = useSetDomainRedirect(appName, domain)
  const clearRedirect = useClearDomainRedirect(appName, domain)

  if (isLoading) {
    return (
      <div
        className="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
        aria-hidden="true"
      >
        <Skeleton className="h-5 w-28 rounded-full" />
        <Skeleton className="h-7 w-32" />
      </div>
    )
  }

  const active = redirect?.enabled ?? false
  const effectiveTargetURL = targetURL ?? redirect?.target_url ?? ''
  const effectiveStatusCode = statusCode ?? String(redirect?.status_code ?? 301)
  const pending = setRedirect.isPending || clearRedirect.isPending

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const req: SetDomainRedirectRequest = {
      target_url: effectiveTargetURL,
      status_code: Number.parseInt(effectiveStatusCode, 10),
    }
    setRedirect.mutate(req)
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
              <SignpostIcon className="size-3" />
              Redirecting to {redirect?.target_url}
            </Badge>
          ) : (
            <Badge variant="muted" className="shrink-0">
              <SignpostIcon className="size-3" />
              No redirect
            </Badge>
          )}
        </button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setOpen((v) => !v)}
        >
          <SignpostIcon className="size-3.5" />
          {open ? 'Hide' : active ? 'Manage' : 'Add redirect'}
        </Button>
      </div>

      {open ? (
        <form
          onSubmit={(e) => {
            handleSubmit(e)
          }}
          className="mt-3 space-y-3"
        >
          <Field>
            <FieldLabel htmlFor={`redirect-target-${domain}`}>
              Target URL
            </FieldLabel>
            <Input
              id={`redirect-target-${domain}`}
              type="url"
              value={effectiveTargetURL}
              onChange={(e) => setTargetURL(e.target.value)}
              disabled={pending}
              placeholder="https://example.com"
            />
            <FieldDescription>
              Every request to {domain} is redirected here instead of reaching
              its container.
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor={`redirect-status-${domain}`}>
              Redirect type
            </FieldLabel>
            <Select
              value={effectiveStatusCode}
              onValueChange={(value) => setStatusCode(value)}
              disabled={pending}
            >
              <SelectTrigger id={`redirect-status-${domain}`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="301">301 Permanent (recommended)</SelectItem>
                <SelectItem value="302">302 Temporary</SelectItem>
              </SelectContent>
            </Select>
            <FieldDescription>
              {effectiveStatusCode === '301'
                ? 'Browsers and search engines remember this redirect and stop checking the original domain.'
                : 'Browsers re-check the original domain on future visits, for a redirect you expect to undo.'}
            </FieldDescription>
          </Field>

          {setRedirect.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{setRedirect.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {clearRedirect.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{clearRedirect.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <div className="flex items-center gap-2">
            <Button
              type="submit"
              size="sm"
              disabled={pending || effectiveTargetURL.length === 0}
            >
              {setRedirect.isPending ? 'Saving...' : 'Save'}
            </Button>
            {active ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={() => {
                  clearRedirect.mutate(undefined, {
                    onSuccess: () => {
                      setTargetURL(null)
                      setStatusCode(null)
                    },
                  })
                }}
              >
                {clearRedirect.isPending ? 'Removing...' : 'Remove redirect'}
              </Button>
            ) : null}
          </div>
        </form>
      ) : null}
    </div>
  )
}
