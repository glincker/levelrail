import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { DnsHostTile } from './DnsHostChip'
import { apexSuggestion, normalizeDomain, providerLabel } from './dnsSetup'

export interface PickCandidate {
  domain: string
  sources: string[]
  provider?: string
}

/** Step 1: the one question. Known domains first, one input behind "Add another domain". */
export function DnsPickStep({
  kind,
  candidates,
  selected,
  detectedProvider,
  onSelect,
  onUseSuggestion,
}: {
  kind: 'mail' | 'app'
  candidates: PickCandidate[]
  selected: string
  detectedProvider: string
  onSelect: (domain: string) => void
  onUseSuggestion: (apex: string, host: string) => void
}) {
  const { t } = useTranslation('dnsSetup')
  const [adding, setAdding] = useState(candidates.length === 0)
  const [raw, setRaw] = useState('')
  const [invalid, setInvalid] = useState(false)
  const typed = normalizeDomain(raw)
  const suggestion = kind === 'mail' && typed ? apexSuggestion(typed) : null

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!typed) {
      setInvalid(true)
      return
    }
    setInvalid(false)
    onSelect(typed)
  }

  return (
    <div className="space-y-2">
      {candidates.length > 0 && (
        <ul
          aria-label={t('pick.listLabel')}
          className="grid gap-1.5 sm:grid-cols-2"
        >
          {candidates.map((c) => {
            const on = c.domain === selected
            const provider = c.provider || (on ? detectedProvider : '')
            return (
              <li key={c.domain}>
                <button
                  type="button"
                  aria-pressed={on}
                  onClick={() => onSelect(c.domain)}
                  className={cn(
                    'flex w-full items-center gap-2 rounded-md border px-2.5 py-2 text-left text-sm focus-visible:outline-2 focus-visible:outline-ring',
                    on
                      ? 'border-primary bg-primary/5'
                      : 'border-border hover:bg-muted',
                  )}
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-mono">{c.domain}</span>
                    <span className="flex flex-wrap gap-1 pt-0.5">
                      {c.sources.map((s) => (
                        <Badge key={s} variant="muted">
                          {t(`pick.source.${s}`, { defaultValue: s })}
                        </Badge>
                      ))}
                    </span>
                  </span>
                  {provider && (
                    <span className="flex items-center gap-1 text-xs text-muted-foreground">
                      <DnsHostTile provider={provider} size="xs" />
                      <span className="hidden sm:inline">
                        {providerLabel(provider)}
                      </span>
                    </span>
                  )}
                </button>
              </li>
            )
          })}
        </ul>
      )}
      {adding ? (
        <form onSubmit={submit} className="flex flex-wrap items-end gap-2">
          <div className="min-w-0 flex-1 space-y-1">
            <Label htmlFor="dns-new-domain" className="text-xs">
              {t('pick.addLabel')}
            </Label>
            <Input
              id="dns-new-domain"
              autoFocus={candidates.length === 0}
              value={raw}
              placeholder={t('pick.placeholder')}
              aria-invalid={invalid}
              onChange={(e) => {
                setRaw(e.target.value)
                setInvalid(false)
              }}
            />
          </div>
          <Button type="submit" size="sm" variant="outline">
            {t('pick.use')}
          </Button>
          {invalid && (
            <p role="alert" className="basis-full text-xs text-tone-danger">
              {t('pick.invalid')}
            </p>
          )}
          {suggestion && (
            <Button
              type="button"
              size="sm"
              variant="secondary"
              className="h-auto basis-full whitespace-normal text-left"
              onClick={() => onUseSuggestion(suggestion.apex, suggestion.host)}
            >
              {t('pick.apexOffer', suggestion)}
            </Button>
          )}
        </form>
      ) : (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="gap-1"
          onClick={() => setAdding(true)}
        >
          <PlusGlassIcon size="1.1em" />
          {t('pick.addAnother')}
        </Button>
      )}
    </div>
  )
}
