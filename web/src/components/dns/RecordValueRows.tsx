import { useTranslation } from 'react-i18next'
import { PlusIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  CAA_TAGS,
  TYPE_PARTS,
  type ValuePart,
  type ValueParts,
} from '../../lib/dnsRecords'
import type { DnsRecordType } from '../../types/dns'

const NARROW: ValuePart[] = ['priority', 'weight', 'port', 'flags']

function PartInput({
  part,
  value,
  label,
  onChange,
}: {
  part: ValuePart
  value: string
  label: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation('dns')
  if (part === 'tag') {
    return (
      <Select
        value={value || 'issue'}
        onValueChange={(v) => onChange(String(v))}
      >
        <SelectTrigger aria-label={label} className="w-32">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {CAA_TAGS.map((tag) => (
            <SelectItem key={tag} value={tag}>
              {tag}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    )
  }
  if (part === 'text') {
    return (
      <Textarea
        aria-label={label}
        value={value}
        rows={2}
        className="font-mono"
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }
  return (
    <Input
      aria-label={label}
      value={value}
      inputMode={NARROW.includes(part) ? 'numeric' : undefined}
      placeholder={t(`record.placeholder.${part}`)}
      className={
        NARROW.includes(part) ? 'w-20 font-mono' : 'min-w-0 flex-1 font-mono'
      }
      onChange={(e) => onChange(e.target.value)}
    />
  )
}

/** RecordValueRows edits a multi value set, with the fields each record type needs. */
export function RecordValueRows({
  type,
  rows,
  onChange,
}: {
  type: DnsRecordType
  rows: ValueParts[]
  onChange: (rows: ValueParts[]) => void
}) {
  const { t } = useTranslation('dns')
  const parts = TYPE_PARTS[type]
  const single = type === 'CNAME'

  function update(i: number, part: ValuePart, v: string) {
    onChange(rows.map((r, j) => (j === i ? { ...r, [part]: v } : r)))
  }

  return (
    <div className="space-y-2">
      {rows.map((row, i) => (
        <div key={i} className="flex items-start gap-2">
          {parts.map((p) => (
            <PartInput
              key={p}
              part={p}
              value={row[p] ?? ''}
              label={t('record.partLabel', {
                part: t(`record.part.${p}`),
                n: i + 1,
              })}
              onChange={(v) => update(i, p, v)}
            />
          ))}
          {rows.length > 1 ? (
            <Button
              size="icon"
              variant="ghost"
              aria-label={t('record.removeValue', { n: i + 1 })}
              onClick={() => onChange(rows.filter((_, j) => j !== i))}
            >
              <TrashIcon />
            </Button>
          ) : null}
        </div>
      ))}
      {single ? null : (
        <Button
          size="sm"
          variant="outline"
          onClick={() =>
            onChange([
              ...rows,
              type === 'CAA' ? { flags: '0', tag: 'issue' } : {},
            ])
          }
        >
          <PlusIcon />
          {t('record.addValue')}
        </Button>
      )}
    </div>
  )
}
