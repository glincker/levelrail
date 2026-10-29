import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useNodeListOptional } from '../../queries/nodes'
import { useProjectListOptional } from '../../queries/projects'

export const LOCAL_NODE = '__local__'
export const NO_PROJECT = '__none__'

export function Advanced({
  nodeId,
  projectId,
  onNode,
  onProject,
}: Readonly<{
  nodeId: string
  projectId: string
  onNode: (id: string) => void
  onProject: (id: string) => void
}>) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  const nodes = useNodeListOptional().data ?? []
  const projects = useProjectListOptional().data ?? []
  // With several nodes the choice matters, so it sits outside the fold.
  const multiNode = nodes.length > 1
  const nodePicker = (
    <div className="min-w-0 space-y-1.5">
      <Label htmlFor="database-node">{t('flow.advanced.node')}</Label>
      <Select
        value={nodeId}
        onValueChange={(v) => {
          onNode(v ?? LOCAL_NODE)
        }}
      >
        <SelectTrigger id="database-node" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={LOCAL_NODE}>
            {t('flow.advanced.autoNode')}
          </SelectItem>
          {nodes.map((n) => (
            <SelectItem key={n.id} value={n.id}>
              {n.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
  return (
    <>
      {multiNode ? nodePicker : null}
      <div className="min-w-0 rounded-lg border border-border">
        <button
          type="button"
          aria-expanded={open}
          aria-controls="database-advanced"
          onClick={() => {
            setOpen((v) => !v)
          }}
          className="flex w-full items-center justify-between gap-2 px-3 py-2 text-sm font-medium focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
        >
          {t('flow.advanced.title')}
          <ChevronDownGlassIcon
            aria-hidden="true"
            className={cn(
              'size-4.5 transition-transform',
              open && 'rotate-180',
            )}
          />
        </button>
        {open ? (
          <div
            id="database-advanced"
            className="grid min-w-0 gap-4 border-t border-border p-3 sm:grid-cols-2"
          >
            {multiNode ? null : nodePicker}
            <div className="min-w-0 space-y-1.5">
              <Label htmlFor="database-project">
                {t('flow.advanced.project')}
              </Label>
              <Select
                value={projectId}
                onValueChange={(v) => {
                  onProject(v ?? NO_PROJECT)
                }}
              >
                <SelectTrigger id="database-project" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_PROJECT}>
                    {t('flow.advanced.noProject')}
                  </SelectItem>
                  {projects.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        ) : null}
      </div>
    </>
  )
}
