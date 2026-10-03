// One expandable row in the API explorer (routes/settings/api-explorer.tsx):
// method/path/ability summary, and once expanded, path-parameter inputs,
// a request body editor for mutating methods, and a "Try it" button that
// fires a real fetch against this same origin using the browser's own
// session cookie, exactly like every other settings page's own queries.

import { useMemo, useState } from 'react'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { pathParamNames } from '@/lib/apiExplorerPath'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsiblePanel,
} from '@/components/ui/collapsible'
import type { OpenAPIRoute } from '@/queries/openapi'

const methodBadgeVariant: Record<
  string,
  'default' | 'success' | 'warning' | 'destructive' | 'muted'
> = {
  GET: 'success',
  POST: 'default',
  PUT: 'warning',
  PATCH: 'warning',
  DELETE: 'destructive',
}

const bodyMethods = new Set(['POST', 'PUT', 'PATCH'])

interface TryItResult {
  status: number
  ok: boolean
  body: string
  durationMs: number
}

async function sendTryItRequest(
  method: string,
  path: string,
  body: string,
): Promise<TryItResult> {
  const started = performance.now()
  const init: RequestInit = { method }
  if (bodyMethods.has(method) && body.trim()) {
    init.headers = { 'Content-Type': 'application/json' }
    init.body = body
  }
  try {
    const res = await fetch(path, init)
    const text = await res.text()
    let pretty = text
    try {
      pretty = JSON.stringify(JSON.parse(text), null, 2)
    } catch {
      // Not JSON (empty body, plain text error page): show it verbatim.
    }
    return {
      status: res.status,
      ok: res.ok,
      body: pretty,
      durationMs: performance.now() - started,
    }
  } catch (err) {
    return {
      status: 0,
      ok: false,
      body: err instanceof Error ? err.message : String(err),
      durationMs: performance.now() - started,
    }
  }
}

export function ApiExplorerRoute({ route }: { route: OpenAPIRoute }) {
  const [open, setOpen] = useState(false)
  const paramNames = useMemo(() => pathParamNames(route.path), [route.path])
  const [paramValues, setParamValues] = useState<Record<string, string>>({})
  const [requestBody, setRequestBody] = useState(
    route.requestBody ? JSON.stringify(route.requestBody, null, 2) : '',
  )
  const [sending, setSending] = useState(false)
  const [result, setResult] = useState<TryItResult | null>(null)

  const resolvedPath = useMemo(() => {
    let p = route.path
    for (const name of paramNames) {
      const value = paramValues[name]?.trim()
      p = p.replace(
        `{${name}}`,
        value ? encodeURIComponent(value) : `{${name}}`,
      )
    }
    return p
  }, [route.path, paramNames, paramValues])

  const missingParams = paramNames.some((name) => !paramValues[name]?.trim())
  const isMutating = bodyMethods.has(route.method) || route.method === 'DELETE'

  async function handleTryIt() {
    setSending(true)
    setResult(await sendTryItRequest(route.method, resolvedPath, requestBody))
    setSending(false)
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-lg border border-border"
    >
      <CollapsibleTrigger className="flex items-center gap-3 px-3 py-2.5">
        <Badge
          variant={methodBadgeVariant[route.method] ?? 'default'}
          className="w-16 shrink-0 justify-center font-mono"
        >
          {route.method}
        </Badge>
        <code className="min-w-0 flex-1 truncate text-left text-sm">
          {route.path}
        </code>
        <Badge variant="outline" className="shrink-0 font-mono text-[10px]">
          {route.ability}
        </Badge>
        <CaretDownIcon
          className={cn(
            'size-4 shrink-0 text-muted-foreground transition-transform',
            open && 'rotate-180',
          )}
        />
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="space-y-4 border-t border-border px-3 py-3">
          <p className="text-sm text-muted-foreground">
            {route.description || (
              <span className="italic">
                No description generated for this route yet.
              </span>
            )}
          </p>

          {paramNames.length > 0 && (
            <div className="grid gap-3 sm:grid-cols-2">
              {paramNames.map((name) => (
                <div key={name} className="space-y-1">
                  <Label htmlFor={`${route.method}-${route.path}-${name}`}>
                    {name}
                  </Label>
                  <Input
                    id={`${route.method}-${route.path}-${name}`}
                    value={paramValues[name] ?? ''}
                    onChange={(e) =>
                      setParamValues((prev) => ({
                        ...prev,
                        [name]: e.target.value,
                      }))
                    }
                    placeholder={name}
                  />
                </div>
              ))}
            </div>
          )}

          {bodyMethods.has(route.method) && (
            <div className="space-y-1">
              <Label>Request body (JSON)</Label>
              <Textarea
                value={requestBody}
                onChange={(e) => setRequestBody(e.target.value)}
                rows={6}
                className="font-mono text-xs"
                placeholder="{}"
              />
            </div>
          )}

          {route.responseBody !== undefined && (
            <details className="text-xs text-muted-foreground">
              <summary className="cursor-pointer select-none">
                Example response
              </summary>
              <pre className="mt-1 overflow-x-auto rounded-md bg-muted p-2">
                {JSON.stringify(route.responseBody, null, 2)}
              </pre>
            </details>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              onClick={() => void handleTryIt()}
              disabled={sending || missingParams}
            >
              {sending ? 'Sending…' : 'Try it'}
            </Button>
            <code className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
              {route.method} {resolvedPath}
            </code>
          </div>
          {isMutating && (
            <p className="text-xs text-amber-700 dark:text-amber-400">
              This sends a real {route.method} request to this instance, not a
              simulation.
            </p>
          )}

          {result && (
            <div className="space-y-1">
              <Badge variant={result.ok ? 'success' : 'destructive'}>
                {result.status || 'network error'} ·{' '}
                {Math.round(result.durationMs)}ms
              </Badge>
              <pre className="max-h-80 overflow-auto rounded-md bg-muted p-2 text-xs">
                {result.body}
              </pre>
            </div>
          )}
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}
