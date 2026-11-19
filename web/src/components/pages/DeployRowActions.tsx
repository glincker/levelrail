import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, ExternalLinkGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  pagesDeploysQueryOptions,
  usePromoteDeploy,
} from '../../queries/pagesDeploys'

/** Copy and open links for one deploy's own URL, plus Promote for a past deploy. Loaded lazily by RecentDeploys. */
export default function DeployRowActions({
  name,
  deployId,
}: Readonly<{ name: string; deployId: string }>) {
  const { t } = useTranslation('pagesSites')
  const { data } = useQuery(pagesDeploysQueryOptions(name))
  const promote = usePromoteDeploy(name)
  const [confirming, setConfirming] = useState(false)
  const [copied, setCopied] = useState(false)
  const entry = data?.find((d) => d.id === deployId)
  if (!entry) return null
  const url = entry.url
  return (
    <span className="flex shrink-0 items-center gap-1">
      {url ? (
        <>
          <Button
            size="xs"
            variant="ghost"
            aria-label={copied ? t('deployUrl.copied') : t('deployUrl.copy')}
            title={url}
            onClick={() => {
              void navigator.clipboard.writeText(url).then(() => {
                setCopied(true)
                setTimeout(() => setCopied(false), 1500)
              })
            }}
          >
            <CopyGlassIcon aria-hidden="true" />
          </Button>
          <Button
            size="xs"
            variant="ghost"
            aria-label={t('deployUrl.open')}
            render={<a href={url} target="_blank" rel="noopener noreferrer" />}
          >
            <ExternalLinkGlassIcon aria-hidden="true" />
          </Button>
        </>
      ) : null}
      {entry.promotable ? (
        <Button size="xs" variant="outline" onClick={() => setConfirming(true)}>
          {t('promote.action')}
        </Button>
      ) : null}
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('promote.confirmTitle', { name })}
        description={t('promote.confirmBody')}
        confirmLabel={t('promote.action')}
        pending={promote.isPending}
        error={promote.error?.message ?? null}
        onConfirm={() =>
          promote.mutate(deployId, { onSuccess: () => setConfirming(false) })
        }
      />
    </span>
  )
}
