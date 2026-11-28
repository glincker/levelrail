import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { NativeSelect } from '@/components/ui/native-select'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { useNodeListOptional } from '../queries/nodes'
import type { NodeResource } from '../types/nodeDetail'

export const AUTO_NODE = ''

function nodeLabel(n: NodeResource, t: TFunction<'nodes'>): string {
  const tags: string[] = []
  if (n.agent?.arch) tags.push(n.agent.arch)
  if (n.gpu?.present && n.gpu.gpu_count > 0) {
    tags.push(t('nodePicker.gpu', { count: n.gpu.gpu_count }))
  }
  if (n.status !== 'online') tags.push(t(`nodePicker.status.${n.status}`))
  return tags.length > 0 ? `${n.name} (${tags.join(', ')})` : n.name
}

/** Placement choice that defaults to auto; hidden when there is nothing to choose between. */
export function NodePicker({
  id,
  value,
  onChange,
}: {
  id: string
  value: string
  onChange: (nodeId: string) => void
}) {
  const { t } = useTranslation('nodes')
  const { data } = useNodeListOptional()
  const nodes = (Array.isArray(data) ? data : []).filter(
    (n) => n.accepts_app_workloads,
  )
  if (nodes.length < 2) return null
  return (
    <Field>
      <FieldLabel htmlFor={id}>{t('nodePicker.label')}</FieldLabel>
      <NativeSelect
        id={id}
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      >
        <option value={AUTO_NODE}>{t('nodePicker.auto')}</option>
        {nodes.map((n) => (
          <option key={n.id} value={n.id} disabled={!n.schedulable}>
            {nodeLabel(n, t)}
          </option>
        ))}
      </NativeSelect>
      <FieldHint>{t('nodePicker.hint')}</FieldHint>
    </Field>
  )
}
