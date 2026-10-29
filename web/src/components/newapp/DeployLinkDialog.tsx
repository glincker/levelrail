import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useBrand } from '../../hooks/useBrand'
import {
  buildRepoLinkUrl,
  repoLinkHtml,
  repoLinkMarkdown,
} from '../../lib/deployRepoLink'

function CopyRow({
  label,
  value,
  mono = true,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  const { t } = useTranslation('newApp')
  const [copied, setCopied] = useState(false)
  const copy = () => {
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      window.setTimeout(() => {
        setCopied(false)
      }, 2000)
    })
  }
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{label}</span>
        <Button type="button" variant="outline" size="sm" onClick={copy}>
          {copied ? (
            <CheckGlassIcon size="1.2em" aria-hidden />
          ) : (
            <CopyGlassIcon size="1.2em" aria-hidden />
          )}
          {copied ? t('deployLinkDialog.copied') : t('deployLinkDialog.copy')}
        </Button>
      </div>
      <pre
        className={`overflow-x-auto rounded-md bg-muted p-3 text-xs break-all whitespace-pre-wrap ${mono ? 'font-mono' : ''}`}
        aria-label={label}
      >
        {value}
      </pre>
    </div>
  )
}

/**
 * Shows a Deploy button link for this dashboard plus Markdown and HTML
 * snippets. With no `repo` it asks for one; the link only prefills.
 */
export function DeployLinkDialog({
  open,
  onOpenChange,
  repo = '',
  branch = '',
  name = '',
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  repo?: string
  branch?: string
  name?: string
}) {
  const { t } = useTranslation('newApp')
  const brand = useBrand()
  const [repoIn, setRepoIn] = useState(repo)
  const [branchIn, setBranchIn] = useState(branch)
  const [nameIn, setNameIn] = useState(name)
  const askRepo = repo === ''
  const built = useMemo(
    () =>
      buildRepoLinkUrl(window.location.origin, {
        repo: repoIn,
        branch: branchIn,
        name: nameIn,
      }),
    [repoIn, branchIn, nameIn],
  )
  const alt = t('deployLinkDialog.alt', { brand: brand.Name })
  const bad = (k: string) => built.invalid.includes(k)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('deployLinkDialog.title')}</DialogTitle>
          <DialogDescription>
            {t('deployLinkDialog.description')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            {askRepo ? (
              <Field className="sm:col-span-2">
                <FieldLabel htmlFor="dld-repo">
                  {t('deployLinkDialog.repo')}
                </FieldLabel>
                <Input
                  id="dld-repo"
                  className="font-mono"
                  placeholder="https://github.com/you/app"
                  value={repoIn}
                  onChange={(e) => {
                    setRepoIn(e.target.value)
                  }}
                />
                {bad('repo') && repoIn !== '' ? (
                  <FieldError>{t('deployLinkDialog.errors.repo')}</FieldError>
                ) : null}
              </Field>
            ) : null}
            <Field>
              <FieldLabel htmlFor="dld-branch">
                {t('deployLinkDialog.branch')}
              </FieldLabel>
              <Input
                id="dld-branch"
                className="font-mono"
                value={branchIn}
                onChange={(e) => {
                  setBranchIn(e.target.value)
                }}
              />
              {bad('branch') ? (
                <FieldError>{t('deployLinkDialog.errors.branch')}</FieldError>
              ) : null}
            </Field>
            <Field>
              <FieldLabel htmlFor="dld-name">
                {t('deployLinkDialog.name')}
              </FieldLabel>
              <Input
                id="dld-name"
                value={nameIn}
                onChange={(e) => {
                  setNameIn(e.target.value)
                }}
              />
              {bad('name') ? (
                <FieldError>{t('deployLinkDialog.errors.name')}</FieldError>
              ) : null}
            </Field>
          </div>
          {built.url !== null ? (
            <>
              <CopyRow label={t('deployLinkDialog.link')} value={built.url} />
              <CopyRow
                label={t('deployLinkDialog.markdown')}
                value={repoLinkMarkdown(alt, built.url)}
              />
              <CopyRow
                label={t('deployLinkDialog.html')}
                value={repoLinkHtml(alt, built.url)}
              />
            </>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
