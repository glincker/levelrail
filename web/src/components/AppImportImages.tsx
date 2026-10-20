import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsLeftRightIcon,
  StopCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  useAppImportImages,
  useCancelAppImportImages,
  useTransferAppImportImages,
  type AppImportImage,
  type AppImportImageState,
  type AppImportSession,
} from '../queries/appImport'
import { formatBytes } from './migrationHubFormat'

const imageVariant: Record<
  AppImportImageState,
  'success' | 'warning' | 'destructive' | 'muted' | 'default'
> = {
  pending: 'muted',
  running: 'default',
  verified: 'success',
  loaded: 'warning',
  failed: 'destructive',
  cancelled: 'muted',
}

function shortID(id?: string): string {
  return id ? id.replace(/^sha256:/, '').slice(0, 12) : ''
}

function ImageRow({ img }: { img: AppImportImage }) {
  const { t } = useTranslation('migration')
  return (
    <li className="space-y-1 rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{img.app}</span>
        <Badge variant={imageVariant[img.state]}>
          {t(`appImport.images.state.${img.state}`)}
        </Badge>
        {img.bytes > 0 ? (
          <span className="text-xs text-muted-foreground tabular-nums">
            {formatBytes(img.bytes)}
          </span>
        ) : null}
      </div>
      <p className="truncate font-mono text-xs text-muted-foreground">
        {img.image}
      </p>
      {img.source_image_id ? (
        <p className="text-xs text-muted-foreground">
          {img.loaded_image_id
            ? t('appImport.images.ids', {
                source: shortID(img.source_image_id),
                loaded: shortID(img.loaded_image_id),
              })
            : t('appImport.images.sourceId', {
                source: shortID(img.source_image_id),
              })}
        </p>
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('appImport.images.noSourceId')}
        </p>
      )}
      {img.error ? (
        <p className="text-xs text-destructive" role="alert">
          {img.error}
        </p>
      ) : null}
    </li>
  )
}

export function AppImportImages({
  session,
  onBack,
  onNext,
}: {
  session: AppImportSession
  onBack: () => void
  onNext: () => void
}) {
  const { t } = useTranslation('migration')
  const list = useAppImportImages(session.id)
  const transfer = useTransferAppImportImages()
  const cancel = useCancelAppImportImages()
  const [ssh, setSsh] = useState('')
  const [key, setKey] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [agent, setAgent] = useState(false)
  const data = list.data
  const running = data?.running ?? false
  const images = data?.images ?? []
  const pending = images.filter(
    (i) => i.state !== 'verified' && i.state !== 'loaded',
  ).length
  const canReuse = data?.credentials_held ?? false

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    transfer.mutate(
      {
        id: session.id,
        body: {
          ssh: ssh.trim() || undefined,
          private_key: key || undefined,
          passphrase: passphrase || undefined,
          use_agent: agent || undefined,
        },
      },
      {
        onSuccess: () => {
          setKey('')
          setPassphrase('')
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {t('appImport.images.title')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('appImport.images.description')}
        </p>
      </div>
      {list.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{t('appImport.images.loadError')}</AlertDescription>
        </Alert>
      ) : null}
      {data && !data.supported ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('appImport.images.unsupported')}
          </AlertDescription>
        </Alert>
      ) : null}
      {data && images.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('appImport.images.none')}
        </p>
      ) : null}
      {images.length > 0 ? (
        <>
          <form onSubmit={onSubmit} className="space-y-3">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="appimport-img-ssh">
                  {t('appImport.images.ssh')}
                </Label>
                <Input
                  id="appimport-img-ssh"
                  required={!canReuse}
                  autoComplete="off"
                  spellCheck={false}
                  placeholder={
                    data?.source ?? t('appImport.images.sshPlaceholder')
                  }
                  value={ssh}
                  onChange={(e) => setSsh(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  {t('appImport.images.sshHelp')}
                </p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="appimport-img-pass">
                  {t('appImport.images.passphrase')}
                </Label>
                <Input
                  id="appimport-img-pass"
                  type="password"
                  autoComplete="off"
                  value={passphrase}
                  disabled={agent}
                  onChange={(e) => setPassphrase(e.target.value)}
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="appimport-img-key">
                {t('appImport.images.key')}
              </Label>
              <Textarea
                id="appimport-img-key"
                className="font-mono text-xs"
                rows={4}
                autoComplete="off"
                spellCheck={false}
                disabled={agent}
                placeholder={
                  canReuse
                    ? t('appImport.images.keyHeld')
                    : t('appImport.images.keyPlaceholder')
                }
                value={key}
                onChange={(e) => setKey(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                {t('appImport.images.keyHelp')}
              </p>
            </div>
            <Label className="flex items-center gap-2 text-sm font-normal">
              <Checkbox
                checked={agent}
                onCheckedChange={(c) => setAgent(c === true)}
              />
              {t('appImport.images.agent')}
            </Label>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="submit"
                disabled={
                  running || transfer.isPending || data?.supported === false
                }
              >
                <ArrowsLeftRightIcon aria-hidden />
                {pending < images.length
                  ? t('appImport.images.retry')
                  : t('appImport.images.move')}
              </Button>
              {running ? (
                <Button
                  type="button"
                  variant="outline"
                  disabled={cancel.isPending}
                  onClick={() => cancel.mutate(session.id)}
                >
                  <StopCircleIcon aria-hidden />
                  {t('appImport.images.cancel')}
                </Button>
              ) : null}
              {running ? (
                <span className="text-xs text-muted-foreground" role="status">
                  {t('appImport.images.running', { source: data?.source })}
                </span>
              ) : null}
            </div>
          </form>
          {transfer.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{transfer.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {cancel.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{cancel.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <ul className="space-y-2" aria-live="polite">
            {images.map((img) => (
              <ImageRow key={img.source_id} img={img} />
            ))}
          </ul>
        </>
      ) : null}
      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={onBack}>
          {t('appImport.back')}
        </Button>
        <Button onClick={onNext} disabled={running}>
          {t('appImport.images.continue')}
        </Button>
        {pending > 0 && !running ? (
          <span className="text-xs text-muted-foreground">
            {t('appImport.images.pending', { count: pending })}
          </span>
        ) : null}
      </div>
    </div>
  )
}
