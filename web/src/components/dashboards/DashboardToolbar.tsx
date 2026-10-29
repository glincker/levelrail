import { useTranslation } from 'react-i18next'
import {
  PlusGlassIcon,
  RefreshCwGlassIcon,
  SaveGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { REFRESH_CHOICES } from '@/lib/dashboardGrid'
import { TIME_RANGE_PRESETS, type TimeRangeKey } from '@/lib/timeRange'
import { useProjectListOptional } from '@/queries/projects'
import type { DashboardScope } from '@/types/dashboards'

const NO_PROJECT = 'none'

export interface Placement {
  scope: DashboardScope
  projectId: string
  isDefault: boolean
}

export function DashboardToolbar({
  editing,
  dirty,
  saving,
  timeRange,
  refreshSeconds,
  placement,
  onTimeRange,
  onRefreshSeconds,
  onRefreshNow,
  onPlacement,
  onDoneEditing,
  onAdd,
  onSave,
}: {
  editing: boolean
  dirty: boolean
  saving: boolean
  timeRange: TimeRangeKey
  refreshSeconds: number
  placement: Placement
  onTimeRange: (k: TimeRangeKey) => void
  onRefreshSeconds: (s: number) => void
  onRefreshNow: () => void
  onPlacement: (p: Placement) => void
  onDoneEditing: () => void
  onAdd: () => void
  onSave: () => void
}) {
  const { t } = useTranslation('dashboards')
  const projects = useProjectListOptional()
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="dash-range">{t('toolbar.range')}</Label>
          <Select
            value={timeRange}
            onValueChange={(v) => {
              if (v) onTimeRange(v)
            }}
          >
            <SelectTrigger id="dash-range" className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TIME_RANGE_PRESETS.map((p) => (
                <SelectItem key={p.key} value={p.key}>
                  {t(`range.${p.key}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="dash-refresh">{t('toolbar.refresh')}</Label>
          <Select
            value={String(refreshSeconds)}
            onValueChange={(v) => {
              if (v) onRefreshSeconds(Number(v))
            }}
          >
            <SelectTrigger id="dash-refresh" className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {REFRESH_CHOICES.map((s) => (
                <SelectItem key={s} value={String(s)}>
                  {t(`refresh.${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button variant="outline" onClick={onRefreshNow}>
          <RefreshCwGlassIcon size="1.2em" data-icon="inline-start" />
          {t('toolbar.refreshNow')}
        </Button>
      </div>
      {editing ? (
        <div className="flex flex-wrap items-end gap-4 rounded-lg border border-border p-3">
          <Button variant="outline" onClick={onAdd}>
            <PlusGlassIcon size="1.2em" data-icon="inline-start" />
            {t('toolbar.addWidget')}
          </Button>
          <div className="flex items-center gap-2">
            <Switch
              id="dash-shared"
              checked={placement.scope === 'shared'}
              onCheckedChange={(on) =>
                onPlacement({
                  ...placement,
                  scope: on ? 'shared' : 'user',
                  isDefault: on ? placement.isDefault : false,
                })
              }
            />
            <Label htmlFor="dash-shared">{t('toolbar.shared')}</Label>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="dash-project">{t('toolbar.project')}</Label>
            <Select
              value={placement.projectId || NO_PROJECT}
              onValueChange={(v) => {
                if (!v) return
                const projectId = v === NO_PROJECT ? '' : v
                onPlacement({
                  ...placement,
                  projectId,
                  isDefault: projectId ? placement.isDefault : false,
                })
              }}
            >
              <SelectTrigger id="dash-project" className="w-44">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_PROJECT}>
                  {t('toolbar.noProject')}
                </SelectItem>
                {(projects.data ?? []).map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <Switch
              id="dash-default"
              checked={placement.isDefault}
              disabled={placement.scope !== 'shared' || !placement.projectId}
              onCheckedChange={(on) =>
                onPlacement({ ...placement, isDefault: on })
              }
            />
            <Label htmlFor="dash-default">{t('toolbar.isDefault')}</Label>
          </div>
          <div className="ml-auto flex gap-2">
            <Button variant="outline" onClick={onDoneEditing}>
              {t('toolbar.done')}
            </Button>
            <Button disabled={!dirty || saving} onClick={onSave}>
              <SaveGlassIcon size="1.2em" data-icon="inline-start" />
              {t('toolbar.save')}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}
