import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'

const URL_PATTERN =
  /^(?:(?:https?|ssh|git):\/\/[^\s/]+\/\S+|[\w.-]+@[\w.-]+:\S+)$/i

/** Paste-a-URL path: any https git URL, with optional branch and access token. */
export function GitUrlForm({
  busy,
  onSource,
}: {
  busy: boolean
  onSource: (source: GitRepoSourceValue) => void
}) {
  const { t } = useTranslation('newApp')
  const [url, setUrl] = useState('')
  const [branch, setBranch] = useState('')
  const [token, setToken] = useState('')
  const urlValid = URL_PATTERN.test(url.trim())

  function submit() {
    if (!urlValid || busy) return
    onSource({
      provider: 'manual',
      repoUrl: url.trim(),
      branch: branch.trim(),
      token: token.trim() || undefined,
    })
  }

  return (
    <div className="space-y-3">
      <Field>
        <FieldLabel htmlFor="new-app-url">{t('source.url.label')}</FieldLabel>
        <Input
          id="new-app-url"
          value={url}
          onChange={(e) => {
            setUrl(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') submit()
          }}
          placeholder="https://github.com/acme/shop"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
        />
        <FieldDescription>{t('source.url.hint')}</FieldDescription>
      </Field>
      <Collapsible>
        <CollapsibleTrigger className="text-xs text-muted-foreground underline underline-offset-2">
          {t('source.url.advanced')}
        </CollapsibleTrigger>
        <CollapsiblePanel className="space-y-3 pt-2">
          <Field>
            <FieldLabel htmlFor="new-app-branch">
              {t('source.url.branch')}
            </FieldLabel>
            <Input
              id="new-app-branch"
              value={branch}
              onChange={(e) => {
                setBranch(e.target.value)
              }}
              placeholder="main"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="new-app-token">
              {t('source.url.token')}
            </FieldLabel>
            <Input
              id="new-app-token"
              type="password"
              value={token}
              onChange={(e) => {
                setToken(e.target.value)
              }}
              autoComplete="off"
            />
            <FieldDescription>{t('source.url.tokenHint')}</FieldDescription>
          </Field>
        </CollapsiblePanel>
      </Collapsible>
      <Button type="button" disabled={!urlValid || busy} onClick={submit}>
        {busy ? t('source.analyzing') : t('source.analyze')}
      </Button>
    </div>
  )
}
