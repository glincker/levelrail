import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useIngressSettings,
  useUpdateIngressSettings,
} from '../queries/domains'

// Shown whenever ingress_settings.primary_domain is unset: every provider's
// OAuth redirect/manifest callback is built from it
// (internal/api/control_plane_base_url.go). Saves it in place so the connect
// flow unlocks without leaving the page. The callout only explains; the field
// sits outside it.
export function SetPrimaryDomainPrompt() {
  const { t } = useTranslation('settings')
  const { data: ingressSettings } = useIngressSettings()
  const update = useUpdateIngressSettings()
  const [domain, setDomain] = useState('')
  const inputId = useId()

  function handleSave() {
    const trimmed = domain.trim()
    if (trimmed === '') {
      return
    }
    update.mutate(
      { ...ingressSettings, primary_domain: trimmed },
      {
        onSuccess: () => {
          toast.add({
            title: t('gitConnection.primaryDomain.saved'),
            type: 'success',
          })
          setDomain('')
        },
        onError: (error) => {
          toast.add({
            title: t('gitConnection.primaryDomain.saveFailed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <Banner
        variant="warning"
        title={t('gitConnection.primaryDomain.title')}
        action={
          <Link
            to="/domains"
            className="text-sm font-medium underline underline-offset-4 hover:no-underline"
          >
            {t('gitConnection.primaryDomain.domainLink')}
          </Link>
        }
      >
        {t('gitConnection.primaryDomain.body')}
      </Banner>
      <Field>
        <FieldLabel htmlFor={inputId}>
          {t('gitConnection.primaryDomain.label')}
        </FieldLabel>
        <div className="flex items-center gap-2">
          <Input
            id={inputId}
            className="font-mono"
            autoComplete="off"
            spellCheck={false}
            placeholder={t('gitConnection.primaryDomain.placeholder')}
            value={domain}
            onChange={(e) => {
              setDomain(e.target.value)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                handleSave()
              }
            }}
          />
          <Button
            type="button"
            disabled={domain.trim() === '' || update.isPending}
            onClick={handleSave}
          >
            {update.isPending
              ? t('gitConnection.primaryDomain.saving')
              : t('gitConnection.primaryDomain.save')}
          </Button>
        </div>
        <FieldDescription>
          {t('gitConnection.primaryDomain.helper')}
        </FieldDescription>
      </Field>
    </div>
  )
}
