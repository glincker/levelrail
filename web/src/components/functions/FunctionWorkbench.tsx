import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { FunctionItem } from '../../queries/functions'
import { FunctionConsole } from './FunctionConsole'
import { InvocationsTable } from './InvocationsTable'

export function FunctionWorkbench({
  items,
}: Readonly<{ items: FunctionItem[] }>) {
  const { t } = useTranslation('functions')
  const [picked, setPicked] = useState('')
  const [tab, setTab] = useState('calls')
  if (items.length === 0) return null
  const name =
    items.find((f) => f.name === picked)?.name ?? items[0]?.name ?? ''

  return (
    <section className="space-y-3" aria-label={t('observe.title')}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-semibold">{t('observe.title')}</h2>
        <Select<string> value={name} onValueChange={(v) => setPicked(v ?? '')}>
          <SelectTrigger className="w-56" aria-label={t('observe.pick')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {items.map((f) => (
              <SelectItem key={f.name} value={f.name}>
                {f.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="calls">{t('observe.tabCalls')}</TabsTrigger>
          <TabsTrigger value="test">{t('observe.tabTest')}</TabsTrigger>
        </TabsList>
        <TabsContent value="calls" className="pt-3">
          <InvocationsTable key={name} name={name} />
        </TabsContent>
        <TabsContent value="test" className="pt-3">
          <FunctionConsole key={name} name={name} />
        </TabsContent>
      </Tabs>
    </section>
  )
}
