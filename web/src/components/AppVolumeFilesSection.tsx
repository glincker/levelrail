import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowUpIcon,
  DownloadSimpleIcon,
  FileIcon,
  FolderIcon,
  TrashIcon,
  UploadSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  useDeleteVolumeFile,
  useUploadVolumeFile,
  useVolumeFiles,
  volumeFileDownloadURL,
} from '../queries/volumeFiles'
import type { AppVolume } from '../types/appDetail'

function parentOf(dir: string): string {
  const parts = dir.split('/').filter(Boolean)
  return parts.slice(0, -1).join('/')
}

function FilesBrowser({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const { t } = useTranslation('backups')
  const [dir, setDir] = useState('')
  const [confirming, setConfirming] = useState<string | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const { data, isLoading, error } = useVolumeFiles(appName, volumeName, dir)
  const upload = useUploadVolumeFile(appName, volumeName)
  const remove = useDeleteVolumeFile(appName, volumeName)
  const failure = error ?? upload.error ?? remove.error

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={dir === ''}
          onClick={() => setDir(parentOf(dir))}
        >
          <ArrowUpIcon aria-hidden />
          {t('volumeFiles.up')}
        </Button>
        <span className="font-mono text-xs text-muted-foreground">/{dir}</span>
        <input
          ref={fileInput}
          type="file"
          className="sr-only"
          aria-label={t('volumeFiles.upload')}
          onChange={(e) => {
            const file = e.target.files?.[0]
            if (file) {
              upload.mutate({
                path: dir ? `${dir}/${file.name}` : file.name,
                file,
              })
            }
            e.target.value = ''
          }}
        />
        <Button
          size="sm"
          className="ml-auto"
          disabled={upload.isPending}
          onClick={() => fileInput.current?.click()}
        >
          <UploadSimpleIcon aria-hidden />
          {t('volumeFiles.upload')}
        </Button>
      </div>
      {failure ? (
        <p role="alert" className="text-sm text-destructive">
          {failure.message}
        </p>
      ) : null}
      {isLoading ? (
        <p className="text-sm text-muted-foreground">
          {t('volumeFiles.loading')}
        </p>
      ) : data && data.entries.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('volumeFiles.empty')}
        </p>
      ) : (
        <ul className="divide-y rounded-lg border">
          {data?.entries.map((e) => (
            <li key={e.path} className="flex items-center gap-2 px-3 py-1.5">
              {e.type === 'dir' ? (
                <FolderIcon aria-hidden className="size-4 shrink-0" />
              ) : (
                <FileIcon aria-hidden className="size-4 shrink-0" />
              )}
              {e.type === 'dir' ? (
                <button
                  type="button"
                  className="min-w-0 truncate text-left text-sm font-medium hover:underline"
                  onClick={() => setDir(e.path)}
                >
                  {e.name}
                </button>
              ) : (
                <span className="min-w-0 truncate text-sm">{e.name}</span>
              )}
              <span className="ml-auto shrink-0 font-mono text-xs text-muted-foreground">
                {e.type === 'file' ? e.size : e.type}
              </span>
              {e.type === 'file' ? (
                <>
                  <a
                    href={volumeFileDownloadURL(appName, volumeName, e.path)}
                    download
                    aria-label={t('volumeFiles.download', { name: e.name })}
                    className="text-muted-foreground hover:text-foreground"
                  >
                    <DownloadSimpleIcon aria-hidden className="size-4" />
                  </a>
                  <Button
                    variant={confirming === e.path ? 'destructive' : 'ghost'}
                    size="sm"
                    aria-label={t('volumeFiles.delete', { name: e.name })}
                    onClick={() => {
                      if (confirming === e.path) {
                        remove.mutate(e.path)
                        setConfirming(null)
                      } else {
                        setConfirming(e.path)
                      }
                    }}
                  >
                    <TrashIcon aria-hidden />
                    {confirming === e.path ? t('volumeFiles.confirm') : null}
                  </Button>
                </>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      {data?.truncated ? (
        <p className="text-xs text-muted-foreground">
          {t('volumeFiles.truncated')}
        </p>
      ) : null}
    </div>
  )
}

export function AppVolumeFilesSection({
  appName,
  volumes,
}: {
  appName: string
  volumes: AppVolume[] | null | undefined
}) {
  const { t } = useTranslation('backups')
  const list = volumes ?? []
  const [picked, setPicked] = useState('')
  if (list.length === 0) return null
  const current = list.find((v) => v.name === picked) ?? list[0]
  if (!current) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('volumeFiles.title')}</CardTitle>
        {list.length > 1 ? (
          <select
            aria-label={t('volumeFiles.volume')}
            className="w-fit rounded-md border bg-background px-2 py-1 text-sm"
            value={current.name}
            onChange={(e) => setPicked(e.target.value)}
          >
            {list.map((v) => (
              <option key={v.name} value={v.name}>
                {v.name}
              </option>
            ))}
          </select>
        ) : null}
      </CardHeader>
      <CardContent>
        <FilesBrowser
          key={current.name}
          appName={appName}
          volumeName={current.name}
        />
      </CardContent>
    </Card>
  )
}
