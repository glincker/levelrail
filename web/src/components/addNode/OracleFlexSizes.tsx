import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  SelectedMark,
  selectableCardClass,
} from '@/components/ui/selectable-card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  oracleFlexId,
  oracleFlexShapeOf,
  type OracleFlexShape,
} from '../../lib/oracleProvision'

function FlexCard({
  flex,
  value,
  onChange,
}: {
  flex: OracleFlexShape
  value: string
  onChange: (id: string) => void
}) {
  const { t } = useTranslation('nodes')
  const first = flex.presets[0]
  const selectedHere = oracleFlexShapeOf(value) === flex.shape
  const [ocpus, setOcpus] = useState(String(first?.ocpus ?? 1))
  const [memory, setMemory] = useState(String(first?.memoryGb ?? 6))
  const key = (f: string) => `oracle-flex-${flex.shape}-${f}`

  function apply(o: string, m: string) {
    const oc = Number(o)
    const mem = Number(m)
    if (Number.isInteger(oc) && Number.isInteger(mem) && oc > 0 && mem > 0) {
      onChange(oracleFlexId(flex.shape, oc, mem))
    }
  }

  return (
    <div
      className={selectableCardClass(
        selectedHere,
        'space-y-2 rounded-lg p-3 pr-7 text-sm',
      )}
    >
      <SelectedMark selected={selectedHere} />
      <p className="font-medium">
        {flex.shape}{' '}
        <span className="text-xs font-normal text-muted-foreground">
          {t('oracle.sizes.flex')}
        </span>
      </p>
      <div className="flex flex-wrap items-end gap-2">
        {flex.presets.map((p) => (
          <button
            key={p.id}
            type="button"
            aria-pressed={value === p.id}
            onClick={() => {
              setOcpus(String(p.ocpus))
              setMemory(String(p.memoryGb))
              onChange(p.id)
            }}
            className={selectableCardClass(
              value === p.id,
              'rounded-md px-2 py-1 text-xs',
            )}
          >
            {t('oracle.sizes.preset', { ocpus: p.ocpus, memory: p.memoryGb })}
          </button>
        ))}
        <Field className="w-24">
          <FieldLabel htmlFor={key('ocpu')}>
            {t('oracle.sizes.ocpus')}
          </FieldLabel>
          <Input
            id={key('ocpu')}
            type="number"
            min={1}
            step={1}
            inputMode="numeric"
            value={ocpus}
            onChange={(e) => {
              setOcpus(e.target.value)
              apply(e.target.value, memory)
            }}
          />
        </Field>
        <Field className="w-28">
          <FieldLabel htmlFor={key('mem')}>
            {t('oracle.sizes.memory')}
          </FieldLabel>
          <Input
            id={key('mem')}
            type="number"
            min={1}
            step={1}
            inputMode="numeric"
            value={memory}
            onChange={(e) => {
              setMemory(e.target.value)
              apply(ocpus, e.target.value)
            }}
          />
        </Field>
      </div>
    </div>
  )
}

export function OracleFlexSizes({
  shapes,
  value,
  onChange,
}: {
  shapes: OracleFlexShape[]
  value: string
  onChange: (id: string) => void
}) {
  return (
    <>
      {shapes.map((f) => (
        <FlexCard key={f.shape} flex={f} value={value} onChange={onChange} />
      ))}
    </>
  )
}
