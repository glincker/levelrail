import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { RelativeTime, StatusPill, type Tone } from '@/components/kit'
import {
  useImageLayers,
  useVerifyImage,
  type ImageEntry,
  type ImageVerifyStatus,
} from '../../queries/imageInventory'
import {
  imageTitle,
  lineageOf,
  shortId,
  sizeLabel,
  sourceKey,
} from './imageFormat'

const VERIFY_TONE: Record<ImageVerifyStatus, Tone> = {
  verified: 'success',
  unsigned: 'warning',
  failed: 'danger',
  not_configured: 'neutral',
  no_registry_digest: 'neutral',
}

interface Props {
  node: string
  image: ImageEntry | null
  onClose: () => void
}

export function ImageDetailSheet({ node, image, onClose }: Props) {
  const { t } = useTranslation('images')
  const layers = useImageLayers(node, image?.id ?? null)
  const verify = useVerifyImage(node)
  const lineage = image ? lineageOf(image) : []

  return (
    <Sheet
      open={image !== null}
      onOpenChange={(open) => {
        if (!open) {
          verify.reset()
          onClose()
        }
      }}
    >
      <SheetContent className="overflow-y-auto sm:max-w-xl">
        {image ? (
          <>
            <SheetHeader className="border-b border-border pr-12">
              <SheetTitle className="break-all">
                {imageTitle(image) ?? t('untagged')}
              </SheetTitle>
              <SheetDescription className="font-mono text-xs break-all">
                {image.digest
                  ? `${t('detail.digest')}: ${image.digest}`
                  : `${shortId(image.id)}, ${t('detail.localBuild')}`}
              </SheetDescription>
            </SheetHeader>

            <div className="flex flex-col gap-6 p-4">
              <section className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={verify.isPending}
                    onClick={() => {
                      verify.mutate(image.id)
                    }}
                  >
                    <ShieldCheckGlassIcon
                      className="text-tone-success"
                      size="1.4em"
                      aria-hidden="true"
                    />
                    {verify.isPending
                      ? t('detail.verifying')
                      : t('detail.verify')}
                  </Button>
                  {verify.data ? (
                    <StatusPill
                      tone={VERIFY_TONE[verify.data.status]}
                      label={t(`verify.${verify.data.status}`)}
                      title={verify.data.message}
                    />
                  ) : null}
                </div>
                {verify.data?.status === 'not_configured' ? (
                  <p className="text-xs text-muted-foreground">
                    {t('verify.fix')}
                  </p>
                ) : null}
                {verify.data &&
                verify.data.status !== 'verified' &&
                verify.data.status !== 'not_configured' ? (
                  <p className="text-xs text-muted-foreground">
                    {verify.data.message}
                  </p>
                ) : null}
              </section>

              <section className="flex flex-col gap-2">
                <h3 className="text-sm font-medium">{t('detail.lineage')}</h3>
                {lineage.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    {t('detail.lineageEmpty')}
                  </p>
                ) : (
                  <ol className="flex flex-col gap-2">
                    {lineage.map((use) => (
                      <li
                        key={`${use.app}-${use.deploy_id}`}
                        className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-border px-3 py-2 text-sm"
                      >
                        <span className="font-medium">{use.app}</span>
                        <StatusPill
                          size="sm"
                          tone={
                            sourceKey(use.source) === 'promote'
                              ? 'accent'
                              : 'neutral'
                          }
                          label={t(`sources.${sourceKey(use.source)}`)}
                        />
                        {use.branch ? (
                          <span className="font-mono text-xs text-muted-foreground">
                            {use.branch}
                          </span>
                        ) : null}
                        {use.commit_sha ? (
                          <span className="font-mono text-xs text-muted-foreground">
                            {use.commit_sha.slice(0, 7)}
                          </span>
                        ) : null}
                        <span className="ml-auto text-xs text-muted-foreground">
                          {use.at ? <RelativeTime at={use.at} /> : null}
                        </span>
                      </li>
                    ))}
                  </ol>
                )}
              </section>

              <section className="flex flex-col gap-2">
                <h3 className="text-sm font-medium">{t('detail.layers')}</h3>
                {layers.data && layers.data.layers.length > 0 ? (
                  <ul className="flex flex-col gap-2">
                    {layers.data.layers
                      .filter((l) => l.size_bytes > 0)
                      .map((l) => {
                        const total = layers.data.size_bytes
                        const percent =
                          total > 0
                            ? Math.round((l.size_bytes / total) * 100)
                            : 0
                        return (
                          <li
                            key={l.id + l.created_by}
                            className="flex flex-col gap-1"
                          >
                            <div className="flex items-baseline justify-between gap-3">
                              <code className="truncate text-xs">
                                {l.created_by}
                              </code>
                              <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                                {t('detail.layerSize', {
                                  size: sizeLabel(l.size_bytes),
                                  percent,
                                })}
                              </span>
                            </div>
                            <Progress
                              value={percent}
                              aria-label={l.created_by}
                            />
                          </li>
                        )
                      })}
                  </ul>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    {layers.isPending ? '' : t('detail.layersEmpty')}
                  </p>
                )}
              </section>
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
