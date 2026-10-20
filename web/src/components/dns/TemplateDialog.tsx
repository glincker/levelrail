import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  dnsTemplatesQueryOptions,
  useApplyDnsTemplate,
} from '../../queries/dns'
import type { DnsPlanResult } from '../../types/dns'
import { PlanView } from './PlanView'

/** TemplateDialog renders a built in record template as a plan, then applies it. */
export function TemplateDialog({
  zone,
  onClose,
}: {
  zone: string
  onClose: () => void
}) {
  const { t } = useTranslation('dns')
  const templates = useQuery(dnsTemplatesQueryOptions())
  const apply = useApplyDnsTemplate(zone)
  const [id, setId] = useState('')
  const [params, setParams] = useState<Record<string, string>>({})
  const [plan, setPlan] = useState<DnsPlanResult | null>(null)
  const tpl = templates.data?.find((x) => x.id === id)

  function run(doApply: boolean) {
    apply.mutate(
      { id, params, apply: doApply },
      {
        onSuccess: (res) => {
          setPlan(res)
          if (doApply) {
            toast.add({
              title: t('import.appliedToast', { count: res.applied }),
              type: 'success',
            })
            onClose()
          }
        },
        onError: (err) =>
          toast.add({
            title: t('templates.failedToast'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Dialog open onOpenChange={(o) => (o ? undefined : onClose())}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('templates.title')}</DialogTitle>
          <DialogDescription>{t('templates.lead')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Select
            value={id}
            onValueChange={(v) => {
              setId(String(v))
              setParams({})
              setPlan(null)
            }}
          >
            <SelectTrigger aria-label={t('templates.pick')} className="w-full">
              <SelectValue placeholder={t('templates.pick')} />
            </SelectTrigger>
            <SelectContent>
              {(templates.data ?? []).map((x) => (
                <SelectItem key={x.id} value={x.id}>
                  {x.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {tpl ? (
            <p className="text-sm text-muted-foreground">{tpl.description}</p>
          ) : null}
          {(tpl?.params ?? []).map((p) => (
            <div key={p.key} className="space-y-1.5">
              <Label htmlFor={`dns-tpl-${p.key}`}>
                {p.label}
                {p.required ? ' *' : ''}
              </Label>
              <Input
                id={`dns-tpl-${p.key}`}
                value={params[p.key] ?? ''}
                placeholder={p.placeholder ?? p.default}
                onChange={(e) => {
                  setParams({ ...params, [p.key]: e.target.value })
                  setPlan(null)
                }}
              />
            </div>
          ))}
          {plan ? <PlanView result={plan} /> : null}
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              disabled={!tpl || apply.isPending}
              onClick={() => run(false)}
            >
              {t('import.preview')}
            </Button>
            <Button
              disabled={!plan || plan.plan.blocked || apply.isPending}
              onClick={() => run(true)}
            >
              {t('import.apply')}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
