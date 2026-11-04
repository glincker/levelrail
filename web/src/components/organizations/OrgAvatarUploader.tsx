import { useRef, useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon, UploadGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { cn } from '@/lib/utils'
import {
  ORG_AVATAR_MAX_BYTES,
  ORG_AVATAR_TYPES,
  useRemoveOrgAvatar,
  useUploadOrgAvatar,
} from '../../queries/organizationProfile'
import { OrgAvatar } from './OrgAvatar'

export interface OrgAvatarUploaderProps {
  id: string
  name: string
  avatarUrl?: string
}

function isAllowedType(type: string): boolean {
  return (ORG_AVATAR_TYPES as readonly string[]).includes(type)
}

export function OrgAvatarUploader({
  id,
  name,
  avatarUrl,
}: OrgAvatarUploaderProps) {
  const { t } = useTranslation('settings')
  const inputRef = useRef<HTMLInputElement>(null)
  const upload = useUploadOrgAvatar(id)
  const remove = useRemoveOrgAvatar(id)
  const [error, setError] = useState('')
  const [progress, setProgress] = useState<number | null>(null)
  const [preview, setPreview] = useState('')
  const [dragging, setDragging] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)

  function send(file: File) {
    if (!isAllowedType(file.type)) {
      setError(t('organizationHome.avatar.badType'))
      return
    }
    if (file.size > ORG_AVATAR_MAX_BYTES) {
      setError(
        t('organizationHome.avatar.tooLarge', {
          max: Math.round(ORG_AVATAR_MAX_BYTES / 1024),
        }),
      )
      return
    }
    setError('')
    setProgress(0)
    setPreview(URL.createObjectURL(file))
    upload.mutate(
      { file, onProgress: setProgress },
      {
        onError: (err) => {
          setError(err.message)
          setPreview('')
        },
        onSettled: () => setProgress(null),
        onSuccess: () => setPreview(''),
      },
    )
  }

  function onDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault()
    setDragging(false)
    const file = e.dataTransfer.files[0]
    if (file) send(file)
  }

  const uploading = progress !== null
  return (
    <div className="flex flex-wrap items-center gap-3">
      <div
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={cn(
          'rounded-xl p-1 ring-2 ring-transparent motion-safe:transition-shadow',
          dragging && 'ring-ring',
        )}
      >
        <OrgAvatar
          id={id}
          name={name}
          avatarUrl={preview || avatarUrl}
          size="lg"
        />
      </div>
      <div className="flex min-w-0 flex-col gap-1.5">
        <div className="flex flex-wrap items-center gap-2">
          <input
            ref={inputRef}
            type="file"
            accept={ORG_AVATAR_TYPES.join(',')}
            className="sr-only"
            tabIndex={-1}
            aria-label={t('organizationHome.avatar.input')}
            data-testid="org-avatar-input"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) send(file)
              e.target.value = ''
            }}
          />
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={uploading}
            onClick={() => inputRef.current?.click()}
          >
            <UploadGlassIcon size="1.2em" aria-hidden="true" />
            {t('organizationHome.avatar.upload')}
          </Button>
          {avatarUrl ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={uploading}
              onClick={() => setConfirmOpen(true)}
            >
              <TrashGlassIcon size="1.2em" aria-hidden="true" />
              {t('organizationHome.avatar.remove')}
            </Button>
          ) : null}
        </div>
        <p className="text-xs text-muted-foreground">
          {t('organizationHome.avatar.hint', {
            max: Math.round(ORG_AVATAR_MAX_BYTES / 1024),
          })}
        </p>
        {uploading ? (
          <progress
            className="h-1.5 w-40 accent-foreground"
            max={1}
            value={progress}
            aria-label={t('organizationHome.avatar.uploading')}
          />
        ) : null}
        {error ? (
          <p role="alert" className="text-xs text-tone-danger-fg">
            {error}
          </p>
        ) : null}
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('organizationHome.avatar.removeTitle')}
        description={t('organizationHome.avatar.removeBody')}
        confirmLabel={t('organizationHome.avatar.remove')}
        tone="destructive"
        pending={remove.isPending}
        error={remove.error?.message ?? null}
        onConfirm={() =>
          remove.mutate(undefined, { onSuccess: () => setConfirmOpen(false) })
        }
      />
    </div>
  )
}
