import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { messageOf } from '../../lib/cloudAwsServices'
import { formatAbsoluteTime } from '../../lib/relativeTime'
import {
  useAwsCfDistributions,
  useAwsCfInvalidate,
  useAwsCfInvalidations,
} from '../../queries/cloudCfSsm'
import { StateLine } from './AwsServiceShared'

export function CloudFrontCard({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const dists = useAwsCfDistributions(id)
  const invalidate = useAwsCfInvalidate(id)
  const [did, setDid] = useState<string | null>(null)
  const [paths, setPaths] = useState('')
  const invs = useAwsCfInvalidations(id, did)
  const list = paths
    .split('\n')
    .map((p) => p.trim())
    .filter((p) => p !== '')
  const empty =
    dists.data?.length === 0 ? t('services.cloudfront.empty') : undefined
  return (
    <div className="space-y-2">
      <StateLine pending={dists.isPending} error={dists.error} empty={empty} />
      <ul className="space-y-1">
        {(dists.data ?? []).map((d) => (
          <li key={d.id} className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-mono text-xs">{d.id}</span>
            <span className="text-xs text-muted-foreground">
              {d.aliases[0] ?? d.domain} · {d.status}
              {d.enabled ? '' : ` · ${t('services.cloudfront.disabled')}`}
            </span>
            <Button
              size="sm"
              variant="outline"
              className="ml-auto"
              aria-pressed={did === d.id}
              onClick={() => {
                invalidate.reset()
                setDid(did === d.id ? null : d.id)
              }}
            >
              {t('services.cloudfront.select')}
            </Button>
          </li>
        ))}
      </ul>
      {did === null ? null : (
        <form
          className="space-y-2"
          onSubmit={(e) => {
            e.preventDefault()
            if (list.length === 0) return
            invalidate.mutate(
              { did, paths: list },
              {
                onSuccess: () => {
                  setPaths('')
                },
              },
            )
          }}
        >
          <label className="block text-xs font-medium text-muted-foreground">
            {t('services.cloudfront.paths', { id: did })}
            <textarea
              value={paths}
              rows={3}
              placeholder={t('services.cloudfront.pathsHint')}
              className="mt-1 w-full rounded-md border border-input bg-background p-2 font-mono text-xs"
              onChange={(e) => {
                setPaths(e.target.value)
              }}
            />
          </label>
          <div className="flex items-center gap-2">
            <Button
              type="submit"
              size="sm"
              disabled={invalidate.isPending || list.length === 0}
            >
              {t('services.cloudfront.invalidate')}
            </Button>
            {invalidate.isSuccess ? (
              <span role="status" className="text-xs text-muted-foreground">
                {t('services.cloudfront.created', {
                  id: invalidate.data.id,
                })}
              </span>
            ) : null}
          </div>
          {invalidate.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {messageOf(invalidate.error)}
            </p>
          ) : null}
          <h4 className="text-xs font-medium text-muted-foreground">
            {t('services.cloudfront.invalidations')}
          </h4>
          <StateLine
            pending={invs.isPending}
            error={invs.error}
            empty={
              invs.data?.length === 0
                ? t('services.cloudfront.noInvalidations')
                : undefined
            }
          />
          <ul className="space-y-1">
            {(invs.data ?? []).map((i) => (
              <li key={i.id} className="flex gap-2 text-xs">
                <span className="font-mono">{i.id}</span>
                <span className="text-muted-foreground">
                  {i.status} · {formatAbsoluteTime(i.created_at)}
                </span>
              </li>
            ))}
          </ul>
        </form>
      )}
    </div>
  )
}
