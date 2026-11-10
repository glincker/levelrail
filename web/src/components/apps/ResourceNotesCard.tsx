import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { renderNotesMarkdown } from '../../lib/renderNotesMarkdown'
import {
  useResourceNotes,
  useSetResourceNotes,
  type NotesKindPath,
} from '../../queries/resourceNotes'

const MARKDOWN_CLASSES =
  'max-w-full text-sm text-foreground [&_a]:break-all [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_code]:text-xs [&_h1]:text-base [&_h1]:font-semibold [&_h2]:text-sm [&_h2]:font-semibold [&_li]:my-0.5 [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-2 [&_p:first-child]:mt-0 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-muted [&_pre]:p-2 [&_ul]:list-disc [&_ul]:pl-5'

export function NotesMarkdown({ source }: { source: string }) {
  const html = useMemo(() => renderNotesMarkdown(source), [source])
  return (
    <div
      className={MARKDOWN_CLASSES}
      // Safe: renderNotesMarkdown drops raw HTML and images and only emits http(s) and mailto links.
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}

export function ResourceNotesCard({
  path,
  name,
}: {
  path: NotesKindPath
  name: string
}) {
  const { t } = useTranslation('common')
  const ids = useId()
  const notes = useResourceNotes(path, name)
  const save = useSetResourceNotes(path, name)
  const [draft, setDraft] = useState<{
    description: string
    notes: string
  } | null>(null)
  const [preview, setPreview] = useState(false)
  const saved = notes.data ?? { description: '', notes: '' }

  const onSave = () => {
    if (!draft) return
    save.mutate(draft, {
      onSuccess: () => {
        setDraft(null)
        setPreview(false)
        toast.add({ title: t('resourceNotes.saved'), type: 'success' })
      },
      onError: (err) => {
        toast.add({ title: err.message, type: 'error' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('resourceNotes.title')}</CardTitle>
        <CardDescription>{t('resourceNotes.hint')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {draft === null ? (
          <>
            {saved.description === '' && saved.notes === '' ? (
              <p className="text-sm text-muted-foreground">
                {t('resourceNotes.empty')}
              </p>
            ) : (
              <>
                {saved.description !== '' && (
                  <p className="text-sm font-medium text-foreground">
                    {saved.description}
                  </p>
                )}
                {saved.notes !== '' && <NotesMarkdown source={saved.notes} />}
              </>
            )}
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={notes.isPending}
              onClick={() =>
                setDraft({
                  description: saved.description,
                  notes: saved.notes,
                })
              }
            >
              {t('resourceNotes.edit')}
            </Button>
          </>
        ) : (
          <>
            <div className="space-y-1">
              <Label htmlFor={`${ids}-description`}>
                {t('resourceNotes.descriptionLabel')}
              </Label>
              <Input
                id={`${ids}-description`}
                value={draft.description}
                placeholder={t('resourceNotes.descriptionPlaceholder')}
                onChange={(e) =>
                  setDraft({ ...draft, description: e.target.value })
                }
              />
            </div>
            <div className="space-y-1">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor={`${ids}-notes`}>
                  {t('resourceNotes.notesLabel')}
                </Label>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-pressed={preview}
                  onClick={() => setPreview(!preview)}
                >
                  {preview
                    ? t('resourceNotes.write')
                    : t('resourceNotes.preview')}
                </Button>
              </div>
              {preview ? (
                <div className="min-h-32 rounded-md border border-border p-3">
                  <NotesMarkdown source={draft.notes} />
                </div>
              ) : (
                <Textarea
                  id={`${ids}-notes`}
                  rows={8}
                  className="font-mono text-xs"
                  value={draft.notes}
                  placeholder={t('resourceNotes.notesPlaceholder')}
                  onChange={(e) =>
                    setDraft({ ...draft, notes: e.target.value })
                  }
                />
              )}
              <p className="text-xs text-muted-foreground">
                {t('resourceNotes.noSecrets')}
              </p>
            </div>
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                disabled={save.isPending}
                onClick={onSave}
              >
                {t('resourceNotes.save')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={save.isPending}
                onClick={() => {
                  setDraft(null)
                  setPreview(false)
                }}
              >
                {t('resourceNotes.cancel')}
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
