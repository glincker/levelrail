import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { Input } from '@/components/ui/input'
import { ApiError } from '../../lib/apiError'
import {
  useDeleteNodePool,
  useNodePools,
  useSetNodePool,
} from '../../queries/nodePools'

// "disk=ssd, zone=a" into a selector; null when a pair is malformed.
function parseSelector(raw: string): Record<string, string> | null {
  const out: Record<string, string> = {}
  for (const part of raw.split(',')) {
    const trimmed = part.trim()
    if (trimmed === '') continue
    const [key, value, ...extra] = trimmed.split('=').map((p) => p.trim())
    if (!key || !value || extra.length > 0) return null
    out[key] = value
  }
  return out
}

// Named groups of nodes. An app picks a pool in its placement panel instead
// of repeating the same label selector on every app.
export function NodePoolsCard() {
  const { t } = useTranslation('nodes')
  const pools = useNodePools()
  const save = useSetNodePool()
  const remove = useDeleteNodePool()
  const [name, setName] = useState('')
  const [selector, setSelector] = useState('')
  const [invalid, setInvalid] = useState(false)

  // 501 means this control plane has no multi-node store: nothing to show.
  if (pools.error instanceof ApiError && pools.error.status === 501) return null
  if (pools.isPending) return null

  const mutationError = save.error ?? remove.error
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('pools.title')}</CardTitle>
        <CardDescription>{t('pools.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {pools.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{pools.error.message}</AlertDescription>
          </Alert>
        ) : pools.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('pools.empty')}</p>
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {pools.data.map((p) => (
              <li
                key={p.name}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <span className="text-sm font-medium">{p.name}</span>
                  {Object.entries(p.selector).map(([k, v]) => (
                    <Badge key={k} variant="outline">
                      {k}={v}
                    </Badge>
                  ))}
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={remove.isPending}
                  onClick={() => {
                    remove.mutate(p.name)
                  }}
                >
                  {t('pools.remove', { name: p.name })}
                </Button>
              </li>
            ))}
          </ul>
        )}
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            const parsed = parseSelector(selector)
            setInvalid(parsed === null)
            if (parsed === null || name.trim() === '') return
            save.mutate(
              { name: name.trim(), selector: parsed },
              {
                onSuccess: () => {
                  setName('')
                  setSelector('')
                },
              },
            )
          }}
        >
          <Input
            aria-label={t('pools.nameLabel')}
            placeholder={t('pools.namePlaceholder')}
            value={name}
            className="w-40"
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
          <Input
            aria-label={t('pools.selectorLabel')}
            placeholder={t('pools.selectorPlaceholder')}
            value={selector}
            className="w-64"
            onChange={(e) => {
              setSelector(e.target.value)
            }}
          />
          <Button type="submit" disabled={save.isPending || name.trim() === ''}>
            {t('pools.save')}
          </Button>
        </form>
        {invalid ? (
          <p role="alert" className="text-sm text-destructive">
            {t('pools.selectorInvalid')}
          </p>
        ) : null}
        {mutationError ? (
          <p role="alert" className="text-sm text-destructive">
            {mutationError.message}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
