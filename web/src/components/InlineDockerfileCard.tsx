import { useId, useRef, useState, type ChangeEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  PlusGlassIcon,
  RocketGlassIcon,
  TrashGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useDeployInlineDockerfile } from '../queries/inlineSource'

interface FileRow {
  key: number
  path: string
  content: string
}

// Deploy straight from a pasted or uploaded Dockerfile when there is no repo.
export function InlineDockerfileCard({ appName }: { appName: string }) {
  const { t } = useTranslation('gitSource')
  const deploy = useDeployInlineDockerfile(appName)
  const uid = useId()
  const fileInput = useRef<HTMLInputElement>(null)
  const [dockerfile, setDockerfile] = useState('')
  const [rows, setRows] = useState<FileRow[]>([])
  const [nextKey, setNextKey] = useState(1)

  async function loadFile(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (file) setDockerfile(await file.text())
  }

  function addRow() {
    setRows((r) => [...r, { key: nextKey, path: '', content: '' }])
    setNextKey((k) => k + 1)
  }

  function patchRow(key: number, patch: Partial<FileRow>) {
    setRows((r) => r.map((x) => (x.key === key ? { ...x, ...patch } : x)))
  }

  function start() {
    const files = rows
      .filter((r) => r.path.trim() !== '')
      .map((r) => ({ path: r.path.trim(), content: r.content }))
    deploy.mutate(
      { dockerfile, files },
      {
        onSuccess: () => {
          toast.add({ title: t('inline.started'), type: 'success' })
        },
        onError: (error) => {
          toast.add({
            title: t('inline.failed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <section
      aria-labelledby={`${uid}-title`}
      className="space-y-3 rounded-2xl border border-border p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 id={`${uid}-title`} className="text-sm font-medium">
            {t('inline.title')}
          </h2>
          <p className="text-sm text-muted-foreground">{t('inline.hint')}</p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => fileInput.current?.click()}
        >
          <UploadGlassIcon className="size-4" />
          {t('inline.loadFile')}
        </Button>
        <input
          ref={fileInput}
          type="file"
          className="sr-only"
          tabIndex={-1}
          aria-label={t('inline.loadFile')}
          onChange={(e) => void loadFile(e)}
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor={`${uid}-df`}>{t('inline.dockerfile')}</Label>
        <Textarea
          id={`${uid}-df`}
          value={dockerfile}
          onChange={(e) => setDockerfile(e.target.value)}
          placeholder={t('inline.placeholder')}
          spellCheck={false}
          className="min-h-32 font-mono text-xs"
        />
      </div>

      {rows.length > 0 ? (
        <ul className="space-y-2">
          {rows.map((r, i) => (
            <li key={r.key} className="flex flex-wrap items-start gap-2">
              <div className="min-w-40 flex-1">
                <Label htmlFor={`${uid}-p${r.key}`} className="sr-only">
                  {t('inline.filePath', { n: i + 1 })}
                </Label>
                <Input
                  id={`${uid}-p${r.key}`}
                  value={r.path}
                  onChange={(e) => patchRow(r.key, { path: e.target.value })}
                  placeholder={t('inline.pathPlaceholder')}
                  className="font-mono text-xs"
                />
              </div>
              <div className="min-w-56 flex-[2]">
                <Label htmlFor={`${uid}-c${r.key}`} className="sr-only">
                  {t('inline.fileContent', { n: i + 1 })}
                </Label>
                <Textarea
                  id={`${uid}-c${r.key}`}
                  value={r.content}
                  onChange={(e) => patchRow(r.key, { content: e.target.value })}
                  spellCheck={false}
                  className="min-h-9 font-mono text-xs"
                />
              </div>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                aria-label={t('inline.removeFile', { n: i + 1 })}
                onClick={() => setRows((x) => x.filter((y) => y.key !== r.key))}
              >
                <TrashGlassIcon className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      ) : null}

      {deploy.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {deploy.error.message}
        </p>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button type="button" size="sm" variant="outline" onClick={addRow}>
          <PlusGlassIcon className="size-4" />
          {t('inline.addFile')}
        </Button>
        <div className="flex items-center gap-3">
          {deploy.isSuccess ? (
            <p role="status" className="text-sm text-muted-foreground">
              {t('inline.deployId', { id: deploy.data.id ?? '' })}{' '}
              <Link
                to="/apps/$name/deploys"
                params={{ name: appName }}
                className="underline"
              >
                {t('connect.viewDeploy')}
              </Link>
            </p>
          ) : null}
          <Button
            type="button"
            size="sm"
            onClick={start}
            disabled={deploy.isPending || dockerfile.trim() === ''}
          >
            <RocketGlassIcon className="size-4" />
            {t('inline.deploy')}
          </Button>
        </div>
      </div>
    </section>
  )
}
