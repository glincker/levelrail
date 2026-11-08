import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  stackTrustsQueryOptions,
  useAddStackTrust,
  useDeleteStackTrust,
} from '../../queries/stacks'

const VERBS = ['plan', 'apply'] as const
type Verb = (typeof VERBS)[number]

// Mirrors the app CI trusts card; the server enforces the apply bar (exact ref, environment).
export function StackTrustCard({
  name,
  isRoot,
}: Readonly<{ name: string; isRoot: boolean }>) {
  const { t } = useTranslation('stacks')
  const { data, isError } = useQuery({
    ...stackTrustsQueryOptions(name),
    enabled: isRoot,
  })
  const trusts = Array.isArray(data) ? data : []
  const add = useAddStackTrust(name)
  const remove = useDeleteStackTrust(name)
  const [repository, setRepository] = useState('')
  const [ref, setRef] = useState('refs/heads/main')
  const [environment, setEnvironment] = useState('')
  const [verb, setVerb] = useState<Verb>('plan')

  if (!isRoot) return null

  function fail(title: string, e: Error) {
    toast.add({ title, description: e.message, type: 'error' })
  }

  function submit() {
    add.mutate(
      {
        provider: 'github',
        repository: repository.trim(),
        ref_pattern: ref.trim(),
        environment: environment.trim(),
        verbs: [verb],
      },
      {
        onSuccess: () => {
          setRepository('')
          toast.add({ title: t('trust.added'), type: 'success' })
        },
        onError: (e) => {
          fail(t('trust.addFailed'), e)
        },
      },
    )
  }

  return (
    <section
      className="space-y-3 rounded-xl border border-border bg-card p-3 shadow-sm"
      aria-label={t('trust.title')}
    >
      <div>
        <h3 className="text-sm font-semibold">{t('trust.title')}</h3>
        <p className="text-xs text-muted-foreground">{t('trust.hint')}</p>
      </div>
      {isError ? (
        <p className="text-sm text-destructive">{t('trust.loadFailed')}</p>
      ) : trusts.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('trust.empty')}</p>
      ) : (
        <ul className="divide-y divide-border">
          {trusts.map((tr) => (
            <li
              key={tr.id}
              className="flex items-center justify-between gap-3 py-1.5 text-sm"
            >
              <span className="min-w-0 truncate">
                <span className="font-mono">{tr.repository}</span>{' '}
                <span className="text-xs text-muted-foreground">
                  {tr.ref_pattern}
                  {tr.environment ? ` / ${tr.environment}` : ''} /{' '}
                  {tr.verbs.join(', ')}
                </span>
              </span>
              <Button
                variant="ghost"
                size="icon"
                aria-label={t('trust.remove', { repo: tr.repository })}
                disabled={remove.isPending}
                onClick={() => {
                  remove.mutate(tr.id, {
                    onError: (e) => {
                      fail(t('trust.removeFailed'), e)
                    },
                  })
                }}
              >
                <TrashGlassIcon className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5 lg:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="grid gap-1">
          <Label htmlFor={`trust-repo-${name}`} className="text-xs">
            {t('trust.repository')}
          </Label>
          <Input
            id={`trust-repo-${name}`}
            value={repository}
            placeholder="owner/repo"
            onChange={(e) => {
              setRepository(e.target.value)
            }}
          />
        </div>
        <div className="grid gap-1">
          <Label htmlFor={`trust-ref-${name}`} className="text-xs">
            {t('trust.ref')}
          </Label>
          <Input
            id={`trust-ref-${name}`}
            value={ref}
            onChange={(e) => {
              setRef(e.target.value)
            }}
          />
        </div>
        <div className="grid gap-1">
          <Label htmlFor={`trust-env-${name}`} className="text-xs">
            {t('trust.environment')}
          </Label>
          <Input
            id={`trust-env-${name}`}
            value={environment}
            onChange={(e) => {
              setEnvironment(e.target.value)
            }}
          />
        </div>
        <div className="grid gap-1">
          <Label htmlFor={`trust-verb-${name}`} className="text-xs">
            {t('trust.verb')}
          </Label>
          <select
            id={`trust-verb-${name}`}
            className="h-9 rounded-md border border-input bg-background px-2 text-sm"
            value={verb}
            onChange={(e) => {
              setVerb(e.target.value === 'apply' ? 'apply' : 'plan')
            }}
          >
            {VERBS.map((v) => (
              <option key={v} value={v}>
                {t(`trust.verbs.${v}`)}
              </option>
            ))}
          </select>
        </div>
        <Button
          type="submit"
          disabled={
            add.isPending || repository.trim() === '' || ref.trim() === ''
          }
        >
          {t('trust.add')}
        </Button>
      </form>
    </section>
  )
}
