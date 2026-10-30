import { useRef, useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { UploadGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { useUploadAsset } from '../../queries/assets'
import { cn } from '@/lib/utils'

type UploadStatus = 'queued' | 'uploading' | 'done' | 'failed'

interface UploadRow {
  id: number
  name: string
  fraction: number
  status: UploadStatus
  error?: string
}

let nextId = 1

function joinKey(folder: string, name: string): string {
  const clean = folder.trim().replace(/^\/+|\/+$/g, '')
  return clean ? `${clean}/${name}` : name
}

export function UploadZone({ bucketId }: Readonly<{ bucketId: string }>) {
  const { t } = useTranslation('assets')
  const upload = useUploadAsset()
  const inputRef = useRef<HTMLInputElement>(null)
  const [folder, setFolder] = useState('')
  const [dragging, setDragging] = useState(false)
  const [rows, setRows] = useState<UploadRow[]>([])

  function patch(id: number, change: Partial<UploadRow>) {
    setRows((prev) => prev.map((r) => (r.id === id ? { ...r, ...change } : r)))
  }

  async function run(files: File[]) {
    const entries = files.map((file) => ({ file, id: nextId++ }))
    setRows((prev) => [
      ...prev,
      ...entries.map<UploadRow>(({ file, id }) => ({
        id,
        name: file.name,
        fraction: 0,
        status: 'queued',
      })),
    ])
    for (const { file, id } of entries) {
      patch(id, { status: 'uploading' })
      try {
        await upload.mutateAsync({
          bucketId,
          file,
          key: joinKey(folder, file.name),
          onProgress: (fraction) => {
            patch(id, { fraction })
          },
        })
        patch(id, { status: 'done', fraction: 1 })
      } catch (err) {
        patch(id, {
          status: 'failed',
          error: err instanceof Error ? err.message : String(err),
        })
      }
    }
  }

  function onDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault()
    setDragging(false)
    const files = Array.from(e.dataTransfer.files)
    if (files.length > 0) void run(files)
  }

  const finished = rows.some(
    (r) => r.status === 'done' || r.status === 'failed',
  )

  return (
    <section aria-labelledby="asset-upload-title" className="space-y-2">
      <h2 id="asset-upload-title" className="sr-only">
        {t('upload.title')}
      </h2>
      <div
        role="group"
        aria-label={t('upload.zoneLabel')}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => {
          setDragging(false)
        }}
        onDrop={onDrop}
        className={cn(
          'flex flex-wrap items-center gap-3 rounded-lg border border-dashed border-border bg-card px-3 py-2.5 text-sm',
          dragging && 'border-ring bg-muted/60',
        )}
      >
        <UploadGlassIcon
          aria-hidden="true"
          className="size-6 text-muted-foreground"
        />
        <span className="text-muted-foreground">
          {dragging ? t('upload.dropActive') : t('upload.drop')}
        </span>
        <span className="text-muted-foreground">{t('upload.or')}</span>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="sr-only"
          tabIndex={-1}
          aria-hidden="true"
          onChange={(e) => {
            const files = Array.from(e.target.files ?? [])
            e.target.value = ''
            if (files.length > 0) void run(files)
          }}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => inputRef.current?.click()}
        >
          {t('upload.choose')}
        </Button>
        <div className="ml-auto flex items-center gap-2">
          <Label htmlFor="asset-folder" className="text-xs">
            {t('upload.folder')}
          </Label>
          <Input
            id="asset-folder"
            value={folder}
            placeholder={t('upload.folderHint')}
            className="h-7 w-44"
            onChange={(e) => {
              setFolder(e.target.value)
            }}
          />
        </div>
      </div>
      {rows.length > 0 ? (
        <ul className="space-y-1.5" aria-live="polite">
          {rows.map((r) => (
            <li key={r.id} className="flex items-center gap-3 text-xs">
              <span className="w-40 truncate">{r.name}</span>
              {r.status === 'failed' ? (
                <span className="text-destructive">
                  {t('upload.failed', { message: r.error ?? '' })}
                </span>
              ) : r.status === 'done' ? (
                <span className="text-muted-foreground">
                  {t('upload.done')}
                </span>
              ) : r.status === 'queued' ? (
                <span className="text-muted-foreground">
                  {t('upload.queued')}
                </span>
              ) : (
                <Progress
                  className="flex-1"
                  value={Math.round(r.fraction * 100)}
                  aria-label={t('upload.progress', { name: r.name })}
                />
              )}
            </li>
          ))}
          {finished ? (
            <li>
              <Button
                type="button"
                size="xs"
                variant="ghost"
                onClick={() => {
                  setRows((prev) =>
                    prev.filter(
                      (r) => r.status === 'queued' || r.status === 'uploading',
                    ),
                  )
                }}
              >
                {t('upload.clear')}
              </Button>
            </li>
          ) : null}
        </ul>
      ) : null}
    </section>
  )
}
