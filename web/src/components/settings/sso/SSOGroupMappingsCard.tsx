import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import type { SSOGroupMapping } from '@/queries/sso'
import { useSaveSSOMappings } from '@/queries/sso'
import type { RoleResource } from '@/queries/roles'
import { Button } from '@/components/ui/button'
import { FormSection } from '@/components/ui/form-section'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'

interface Row extends SSOGroupMapping {
  key: number
}

/** SSOGroupMappingsCard edits the shared IdP and SCIM group to role mappings. */
export function SSOGroupMappingsCard({
  mappings,
  roles,
}: {
  mappings: SSOGroupMapping[]
  roles: RoleResource[]
}) {
  const { t } = useTranslation('sso')
  const save = useSaveSSOMappings()
  const [rows, setRows] = useState<Row[]>(() =>
    mappings.map((m, i) => ({ ...m, key: i })),
  )
  const [nextKey, setNextKey] = useState(mappings.length)
  const stored = roles.filter((r) => r.id)
  const update = (key: number, patch: Partial<SSOGroupMapping>) =>
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  const add = () => {
    setRows((rs) => [
      ...rs,
      { key: nextKey, group: '', role_id: stored[0]?.id ?? '', priority: 0 },
    ])
    setNextKey((k) => k + 1)
  }

  const submit = () => {
    save.mutate(
      rows.map(({ group, role_id, priority }) => ({
        group: group.trim(),
        role_id,
        priority,
      })),
      {
        onSuccess: () =>
          toast.add({ title: t('mappings.saved'), type: 'success' }),
      },
    )
  }

  return (
    <FormSection
      title={t('mappings.title')}
      description={t('mappings.description')}
      error={save.isError ? save.error.message : undefined}
    >
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('mappings.empty')}</p>
      ) : (
        <div className="space-y-2">
          <div className="hidden grid-cols-[minmax(0,2fr)_minmax(0,2fr)_6rem_2rem] gap-2 text-xs font-medium text-muted-foreground sm:grid">
            <span>{t('mappings.group')}</span>
            <span>{t('mappings.role')}</span>
            <span>{t('mappings.priority')}</span>
          </div>
          {rows.map((r) => (
            <div
              key={r.key}
              className="grid grid-cols-1 gap-2 sm:grid-cols-[minmax(0,2fr)_minmax(0,2fr)_6rem_2rem]"
            >
              <Input
                aria-label={t('mappings.group')}
                value={r.group}
                onChange={(e) => update(r.key, { group: e.target.value })}
              />
              <Select
                value={r.role_id}
                onValueChange={(v) => update(r.key, { role_id: v ?? '' })}
              >
                <SelectTrigger
                  aria-label={t('mappings.role')}
                  className="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {stored.map((role) => (
                    <SelectItem key={role.id} value={role.id ?? ''}>
                      {role.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                aria-label={t('mappings.priority')}
                type="number"
                value={r.priority}
                onChange={(e) =>
                  update(r.key, { priority: Number(e.target.value) || 0 })
                }
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t('mappings.remove', { group: r.group })}
                onClick={() =>
                  setRows((rs) => rs.filter((x) => x.key !== r.key))
                }
              >
                <TrashGlassIcon size="1.2em" aria-hidden="true" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="outline" onClick={add}>
          <PlusGlassIcon size="1.2em" aria-hidden="true" />
          {t('mappings.add')}
        </Button>
        <Button type="button" disabled={save.isPending} onClick={submit}>
          {t('mappings.save')}
        </Button>
      </div>
    </FormSection>
  )
}
