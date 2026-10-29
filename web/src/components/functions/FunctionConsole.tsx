import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SendGlassIcon } from '@thesvg/ui-react'
import { StatusPill } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CodeBlock } from '@/components/ui/code-block'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  useInvokeFunction,
  type InvokeInput,
  type InvokeResult,
} from '../../queries/functionInvocations'
import { formatDuration, parseHeaderLines } from '../../lib/functionFormat'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const

export function FunctionConsole({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('functions')
  const invoke = useInvokeFunction(name)
  const [method, setMethod] = useState<string>('GET')
  const [path, setPath] = useState('/')
  const [headers, setHeaders] = useState('')
  const [body, setBody] = useState('')
  const [result, setResult] = useState<InvokeResult | null>(null)

  const send = () => {
    const input: InvokeInput = {
      method,
      path: path.trim() || '/',
      headers: parseHeaderLines(headers),
      body,
    }
    if (body.trim() !== '' && !('content-type' in lowerKeys(input.headers))) {
      input.headers['Content-Type'] = looksLikeJSON(body)
        ? 'application/json'
        : 'text/plain'
    }
    invoke.mutate(input, {
      onSuccess: setResult,
      onError: (err) => {
        toast.add({
          title: t('console.failed', { message: err.message }),
          type: 'error',
        })
      },
    })
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-[8rem_1fr]">
        <div className="space-y-1.5">
          <Label htmlFor="fn-console-method">{t('console.method')}</Label>
          <Select<string>
            value={method}
            onValueChange={(v) => setMethod(v ?? 'GET')}
          >
            <SelectTrigger id="fn-console-method" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {METHODS.map((m) => (
                <SelectItem key={m} value={m}>
                  {m}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="fn-console-path">{t('console.path')}</Label>
          <Input
            id="fn-console-path"
            value={path}
            onChange={(e) => {
              setPath(e.target.value)
            }}
            className="font-mono"
          />
        </div>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="fn-console-headers">{t('console.headers')}</Label>
          <Textarea
            id="fn-console-headers"
            value={headers}
            rows={4}
            placeholder={t('console.headersPlaceholder')}
            onChange={(e) => {
              setHeaders(e.target.value)
            }}
            className="font-mono text-xs"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="fn-console-body">{t('console.body')}</Label>
          <Textarea
            id="fn-console-body"
            value={body}
            rows={4}
            placeholder={t('console.bodyPlaceholder')}
            onChange={(e) => {
              setBody(e.target.value)
            }}
            className="font-mono text-xs"
          />
        </div>
      </div>
      <Button onClick={send} disabled={invoke.isPending}>
        <SendGlassIcon className="size-5" aria-hidden="true" />
        {invoke.isPending ? t('console.sending') : t('console.send')}
      </Button>
      {invoke.isPending ? (
        <p className="text-sm text-muted-foreground" role="status">
          {t('console.wakeHint')}
        </p>
      ) : null}
      {result ? <ConsoleResult result={result} /> : null}
    </div>
  )
}

function looksLikeJSON(text: string): boolean {
  try {
    JSON.parse(text)
    return true
  } catch {
    return false
  }
}

function lowerKeys(h: Record<string, string>): Record<string, string> {
  return Object.fromEntries(
    Object.entries(h).map(([k, v]) => [k.toLowerCase(), v]),
  )
}

function ConsoleResult({ result }: Readonly<{ result: InvokeResult }>) {
  const { t } = useTranslation('functions')
  const tone =
    result.status >= 500
      ? 'danger'
      : result.status >= 400
        ? 'warning'
        : 'success'
  return (
    <section
      className="space-y-3 rounded-lg border p-4"
      aria-label={t('console.result')}
    >
      <div className="flex flex-wrap items-center gap-2">
        <StatusPill tone={tone} label={String(result.status)} />
        <span className="tabular-nums text-sm">
          {formatDuration(result.duration_ms)}
        </span>
        <StatusPill
          tone={result.cold_start ? 'info' : 'neutral'}
          label={result.cold_start ? t('observe.coldYes') : t('console.warm')}
        />
        {result.input_stored ? (
          <span className="text-xs text-muted-foreground">
            {t('console.stored')}
          </span>
        ) : null}
      </div>
      <dl className="grid gap-x-4 gap-y-1 font-mono text-xs sm:grid-cols-[max-content_1fr]">
        {Object.entries(result.headers).map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="break-all">{v.join(', ')}</dd>
          </div>
        ))}
      </dl>
      <CodeBlock code={result.body} wrap />
      {result.body_truncated ? (
        <p className="text-xs text-muted-foreground">
          {t('console.truncated')}
        </p>
      ) : null}
    </section>
  )
}
