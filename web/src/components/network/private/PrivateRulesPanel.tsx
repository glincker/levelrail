import { useState } from 'react'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../../hooks/useIsRoot'
import {
  useCreatePrivateNetworkRule,
  useDeletePrivateNetworkRule,
  type PrivateNetworkRule,
} from '../../../queries/privateNetworks'

function AppSelect({
  label,
  apps,
  value,
  onChange,
}: {
  label: string
  apps: string[]
  value: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation('privateNetworks')
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      <Select value={value} onValueChange={(v) => onChange(v ?? '')}>
        <SelectTrigger className="w-48" aria-label={label}>
          <SelectValue placeholder={t('rules.pickApp')} />
        </SelectTrigger>
        <SelectContent>
          {apps.map((a) => (
            <SelectItem key={a} value={a}>
              {a}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

export function PrivateRulesPanel({
  rules,
  apps,
}: {
  rules: PrivateNetworkRule[]
  apps: string[]
}) {
  const { t } = useTranslation('privateNetworks')
  const isRoot = useIsRoot()
  const create = useCreatePrivateNetworkRule()
  const remove = useDeletePrivateNetworkRule()
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [note, setNote] = useState('')
  const onError = (err: Error) =>
    toast.add({ title: err.message, type: 'error' })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('rules.title')}</CardTitle>
        <CardDescription>{t('rules.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {isRoot ? (
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              create.mutate(
                { from, to, note },
                {
                  onSuccess: () => {
                    setFrom('')
                    setTo('')
                    setNote('')
                  },
                  onError,
                },
              )
            }}
          >
            <AppSelect
              label={t('rules.from')}
              apps={apps}
              value={from}
              onChange={setFrom}
            />
            <AppSelect
              label={t('rules.to')}
              apps={apps}
              value={to}
              onChange={setTo}
            />
            <div className="space-y-1.5">
              <Label htmlFor="pn-rule-note">{t('rules.note')}</Label>
              <Input
                id="pn-rule-note"
                value={note}
                onChange={(e) => setNote(e.target.value)}
                placeholder={t('rules.notePlaceholder')}
                className="w-56"
              />
            </div>
            <Button
              type="submit"
              disabled={create.isPending || !from || !to || from === to}
            >
              {t('rules.add')}
            </Button>
          </form>
        ) : null}
        {rules.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('rules.empty')}</p>
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {rules.map((r) => (
              <li
                key={r.id}
                className="flex items-center justify-between gap-3 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {r.from} / {r.to}
                  </p>
                  {r.note ? (
                    <p className="truncate text-xs text-muted-foreground">
                      {r.note}
                    </p>
                  ) : null}
                </div>
                {isRoot ? (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={remove.isPending}
                    onClick={() =>
                      remove.mutate(r.id, {
                        onSuccess: () =>
                          toast.add({
                            title: t('rules.removed'),
                            type: 'success',
                          }),
                        onError,
                      })
                    }
                  >
                    {t('rules.remove')}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
