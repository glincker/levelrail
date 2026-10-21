import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useBranchDeploys } from '../queries/branchDeploys'
import {
  useSplitActions,
  type SplitStartBody,
  type SplitVariantInput,
} from '../queries/splitTesting'

const DEFAULT_WEIGHT = 20
const SELECT_CLASS =
  'h-9 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground'

type SourceKind = Exclude<SplitVariantInput['kind'], never>

/** SplitStartForm picks one variant and its share; production keeps the rest. */
export function SplitStartForm({ appName }: { appName: string }) {
  const { t } = useTranslation('splitTesting')
  const { start } = useSplitActions(appName)
  const branches = useBranchDeploys(appName)
  const ready = (branches.data?.deploys ?? []).filter(
    (d) => d.status === 'active' && d.domain,
  )
  const [kind, setKind] = useState<SourceKind>('branch')
  const [branch, setBranch] = useState('')
  const [release, setRelease] = useState('')
  const [label, setLabel] = useState('')
  const [weight, setWeight] = useState(String(DEFAULT_WEIGHT))
  const [cookieName, setCookieName] = useState('')
  const [cookieTtl, setCookieTtl] = useState('')

  const ref = kind === 'branch' ? branch : release.trim()
  const pct = Number(weight)
  const valid = ref !== '' && Number.isInteger(pct) && pct >= 1 && pct <= 100

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!valid) return
    const body: SplitStartBody = {
      variants: [
        {
          kind,
          ref,
          weight: pct,
          ...(label.trim() ? { label: label.trim() } : {}),
        },
      ],
      ...(cookieName.trim() ? { cookie_name: cookieName.trim() } : {}),
      ...(cookieTtl.trim() ? { cookie_ttl: cookieTtl.trim() } : {}),
    }
    start.mutate(body, {
      onSuccess: () =>
        toast.add({ title: t('start.success'), type: 'success' }),
      onError: (error) =>
        toast.add({
          title: t('start.errorTitle'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <form onSubmit={submit} className="space-y-3">
      <h3 className="text-sm font-medium text-foreground">
        {t('start.title')}
      </h3>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1 text-xs text-muted-foreground">
          {t('start.kind')}
          <select
            className={SELECT_CLASS}
            value={kind}
            onChange={(e) => setKind(e.target.value as SourceKind)}
          >
            <option value="branch">{t('start.kindBranch')}</option>
            <option value="release">{t('start.kindRelease')}</option>
          </select>
        </label>
        {kind === 'branch' ? (
          <label className="space-y-1 text-xs text-muted-foreground">
            {t('start.branch')}
            <select
              className={SELECT_CLASS}
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
            >
              <option value="">{t('start.branchPlaceholder')}</option>
              {ready.map((d) => (
                <option key={d.slug} value={d.branch}>
                  {d.branch}
                </option>
              ))}
            </select>
            {ready.length === 0 && !branches.isPending ? (
              <span className="block text-xs text-muted-foreground">
                {t('start.noBranches')}
              </span>
            ) : null}
          </label>
        ) : (
          <label className="space-y-1 text-xs text-muted-foreground">
            {t('start.release')}
            <Input
              value={release}
              placeholder={t('start.releasePlaceholder')}
              onChange={(e) => setRelease(e.target.value)}
            />
          </label>
        )}
        <label className="space-y-1 text-xs text-muted-foreground">
          {t('start.label')}
          <Input value={label} onChange={(e) => setLabel(e.target.value)} />
        </label>
        <label className="space-y-1 text-xs text-muted-foreground">
          {t('start.weight')}
          <Input
            type="number"
            min={1}
            max={100}
            value={weight}
            onChange={(e) => setWeight(e.target.value)}
          />
        </label>
        <label className="space-y-1 text-xs text-muted-foreground">
          {t('start.cookieName')}
          <Input
            value={cookieName}
            onChange={(e) => setCookieName(e.target.value)}
          />
        </label>
        <label className="space-y-1 text-xs text-muted-foreground">
          {t('start.cookieTtl')}
          <Input
            value={cookieTtl}
            onChange={(e) => setCookieTtl(e.target.value)}
          />
          <span className="block text-xs text-muted-foreground">
            {t('start.cookieTtlHelp')}
          </span>
        </label>
      </div>
      <Button type="submit" size="sm" disabled={!valid || start.isPending}>
        {t('start.submit')}
      </Button>
    </form>
  )
}
