import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SectionTitle } from '@/components/ui/typography'
import { useMutation } from '@tanstack/react-query'
import { UploadGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import {
  guessImportSource,
  IMPORT_EXAMPLES,
  IMPORT_SOURCE_LABEL,
} from '../lib/importInput'
import {
  planImport,
  type ImportPlan,
  type ImportPlanRequest,
} from '../queries/imports'

const MAX_UPLOAD_BYTES = 512 * 1024

/**
 * The single smart input at the top of "New app": one field that accepts a
 * repo URL, image, docker run command, compose file or Dockerfile, and asks
 * the server for a plan preview. Nothing is created here.
 */
export function ImportFrontDoor({
  onPlan,
}: {
  onPlan: (plan: ImportPlan, request: ImportPlanRequest) => void
}) {
  const { t } = useTranslation('databases')
  const [text, setText] = useState('')
  const [fileError, setFileError] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const guess = guessImportSource(text)

  const plan = useMutation({
    meta: { errorToast: false },
    mutationFn: (request: ImportPlanRequest) => planImport(request),
    onSuccess: (result, request) => {
      onPlan(result, request)
    },
  })

  function submit() {
    if (!text.trim()) return
    plan.mutate({ text })
  }

  function onFile(file: File | undefined) {
    setFileError(null)
    if (!file) return
    if (file.size > MAX_UPLOAD_BYTES) {
      setFileError(t('wizard.fileTooLarge'))
      return
    }
    void file.text().then(setText)
  }

  return (
    <section className="space-y-2" aria-labelledby="import-front-door-title">
      <SectionTitle as="h3" id="import-front-door-title">
        {t('wizard.importTitle')}
      </SectionTitle>
      <Textarea
        value={text}
        onChange={(e) => {
          setText(e.target.value)
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit()
        }}
        rows={4}
        spellCheck={false}
        aria-label={t('wizard.importLabel')}
        placeholder={t('wizard.importPlaceholder')}
        className="font-mono text-xs"
      />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          onClick={submit}
          disabled={!text.trim() || plan.isPending}
        >
          {plan.isPending ? t('wizard.reading') : t('wizard.previewPlan')}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            fileRef.current?.click()
          }}
        >
          <UploadGlassIcon size="1.4em" aria-hidden="true" />
          {t('wizard.uploadFile')}
        </Button>
        <input
          ref={fileRef}
          type="file"
          className="sr-only"
          aria-label={t('wizard.uploadLabel')}
          onChange={(e) => {
            onFile(e.target.files?.[0])
            e.target.value = ''
          }}
        />
        {guess ? (
          <Badge variant="muted" data-testid="import-guess">
            {t('wizard.looksLike', { source: IMPORT_SOURCE_LABEL[guess] })}
          </Badge>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>{t('wizard.tryLabel')}</span>
        {IMPORT_EXAMPLES.map((ex) => (
          <button
            key={ex.label}
            type="button"
            className="min-h-6 rounded-md border border-border px-2 py-0.5 pointer-coarse:min-h-10 hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
            onClick={() => {
              setText(ex.value)
            }}
          >
            {ex.label}
          </button>
        ))}
      </div>
      {fileError ? (
        <Alert variant="destructive">
          <AlertDescription>{fileError}</AlertDescription>
        </Alert>
      ) : null}
      {plan.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{plan.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </section>
  )
}
