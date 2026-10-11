import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusPill, type Tone } from '@/components/kit'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import {
  serverReadinessQueryOptions,
  type ReadinessCheck,
} from '../../queries/selfUpgrade'
import { CopyCommand } from './CopyCommand'

const STATUS_TONE: Record<ReadinessCheck['status'], Tone> = {
  pass: 'success',
  warn: 'warning',
  fail: 'danger',
  info: 'neutral',
}

export function ServerReadiness() {
  const { t } = useTranslation('updates')
  const [draft, setDraft] = useState('')
  const [domain, setDomain] = useState('')
  const { data, isPending, isError, error, isFetching } = useQuery(
    serverReadinessQueryOptions(domain),
  )

  return (
    <div className="space-y-4">
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setDomain(draft.trim().toLowerCase())
        }}
      >
        <div className="min-w-0 flex-1 space-y-1">
          <label htmlFor="readiness-domain" className="text-xs font-medium">
            {t('readiness.domainLabel')}
          </label>
          <Input
            id="readiness-domain"
            value={draft}
            placeholder="console.example.com"
            onChange={(e) => setDraft(e.target.value)}
          />
        </div>
        <Button type="submit" variant="outline" disabled={isFetching}>
          {isFetching ? t('readiness.checking') : t('readiness.recheck')}
        </Button>
      </form>
      {isPending ? (
        <p className="text-sm text-muted-foreground">
          {t('readiness.checking')}
        </p>
      ) : isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('readiness.failed', { message: error.message })}
        </p>
      ) : (
        <>
          <ul className="space-y-2" aria-label={t('readiness.checksLabel')}>
            {data.checks.map((c) => (
              <li key={c.id} className="flex items-start gap-3 text-sm">
                <StatusPill
                  tone={STATUS_TONE[c.status]}
                  label={t(`readiness.status.${c.status}`)}
                  size="sm"
                />
                <span className="min-w-0 flex-1">
                  <span className="font-medium text-foreground">{c.name}</span>
                  <span className="block text-xs text-muted-foreground">
                    {c.detail}
                  </span>
                  {c.fix && c.status !== 'pass' ? (
                    <span className="block text-xs text-foreground">
                      {c.fix}
                    </span>
                  ) : null}
                </span>
              </li>
            ))}
          </ul>
          <div className="space-y-2 rounded-md bg-muted px-3 py-3">
            <p className="text-sm font-medium">
              {t(`readiness.mode.${data.mode}`)}
            </p>
            <p className="text-sm text-muted-foreground">{data.summary}</p>
            <CopyCommand
              label={t('readiness.nextStep')}
              command={data.next_step}
            />
          </div>
        </>
      )}
    </div>
  )
}
