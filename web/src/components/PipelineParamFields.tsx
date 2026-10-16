import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { PipelineParameter } from '../types/pipelines'

export function PipelineParamFields({
  params,
  values,
  onChange,
}: {
  params: PipelineParameter[]
  values: Record<string, string>
  onChange: (name: string, value: string) => void
}) {
  const { t } = useTranslation('pipelines')
  if (params.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('run.noParameters')}</p>
    )
  }
  return (
    <div className="space-y-3">
      {params.map((p) => {
        const id = `run-param-${p.name}`
        const value = values[p.name] ?? ''
        return (
          <div key={p.name} className="space-y-1.5">
            <Label htmlFor={id}>
              <span className="font-mono">{p.name}</span>
              {p.required ? (
                <span className="ml-1.5 text-xs font-normal text-muted-foreground">
                  {t('run.required')}
                </span>
              ) : null}
            </Label>
            {p.type === 'boolean' ? (
              <Switch
                id={id}
                checked={value === 'true'}
                aria-label={t('run.valueFor', { name: p.name })}
                onCheckedChange={(on) =>
                  onChange(p.name, on ? 'true' : 'false')
                }
              />
            ) : p.type === 'choice' || (p.choices ?? []).length > 0 ? (
              <Select
                value={value}
                onValueChange={(v) => onChange(p.name, v ?? '')}
              >
                <SelectTrigger id={id} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(p.choices ?? []).map((c) => (
                    <SelectItem key={c} value={c}>
                      {c}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                id={id}
                value={value}
                required={p.required}
                onChange={(e) => onChange(p.name, e.target.value)}
              />
            )}
            {p.description ? (
              <p className="text-xs text-muted-foreground">{p.description}</p>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}
