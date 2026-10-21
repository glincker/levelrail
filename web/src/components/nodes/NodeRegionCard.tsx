import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useSetNodeRegion } from '../../queries/nodes'
import type { NodeResource } from '../../types/nodeDetail'
import { InfoTip } from '../kit/InfoTip'

interface Draft {
  region: string
  provider: string
  zone: string
}

function draftOf(node: NodeResource): Draft {
  return {
    region: node.region ?? '',
    provider: node.provider ?? '',
    zone: node.zone ?? '',
  }
}

// Rendered with key={node.id} by its caller so switching nodes remounts it
// instead of needing an effect to resync the draft.
export function NodeRegionCard({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  const setLabels = useSetNodeRegion()
  const [draft, setDraft] = useState<Draft>(() => draftOf(node))
  const saved = draftOf(node)
  const unchanged =
    draft.region === saved.region &&
    draft.provider === saved.provider &&
    draft.zone === saved.zone

  const fields: {
    key: keyof Draft
    label: string
    placeholder: string
    hint?: string
  }[] = [
    {
      key: 'provider',
      label: t('region.provider'),
      placeholder: t('region.providerPlaceholder'),
    },
    {
      key: 'region',
      label: t('region.region'),
      placeholder: t('region.regionPlaceholder'),
      hint: t('region.regionHint'),
    },
    {
      key: 'zone',
      label: t('region.zone'),
      placeholder: t('region.zonePlaceholder'),
      hint: t('region.zoneHint'),
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('region.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {fields.map((f) => (
          <Field key={f.key}>
            <FieldLabel
              htmlFor={`node-${f.key}`}
              className="flex items-center gap-1.5"
            >
              {f.label}
              {f.hint ? <InfoTip label={f.label}>{f.hint}</InfoTip> : null}
            </FieldLabel>
            <Input
              id={`node-${f.key}`}
              value={draft[f.key]}
              placeholder={f.placeholder}
              disabled={setLabels.isPending}
              onChange={(e) => setDraft({ ...draft, [f.key]: e.target.value })}
              className="max-w-xs"
            />
          </Field>
        ))}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={setLabels.isPending || unchanged}
          onClick={() => {
            setLabels.mutate(
              {
                id: node.id,
                region: draft.region.trim(),
                provider: draft.provider.trim(),
                zone: draft.zone.trim(),
              },
              {
                onSuccess: () => {
                  toast.add({ title: t('region.saved'), type: 'success' })
                },
              },
            )
          }}
        >
          {t('region.save')}
        </Button>
        <FieldDescription>{t('region.description')}</FieldDescription>
        {setLabels.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{setLabels.error.message}</AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  )
}
