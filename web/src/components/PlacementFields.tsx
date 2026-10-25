import { useTranslation } from 'react-i18next'
import type { NodeResource } from '../types/nodeDetail'
import type { ProjectResource } from '../types/projectDetail'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

// Sentinel for "this control plane's own local node", the implicit
// default PUT /api/v1/apps/{name}/node's own doc comment establishes
// (empty node_id). Not a real row in the node list, there is no
// separate "local" entry the API returns.
export const LOCAL_NODE_VALUE = ''

// Sentinel for "let the control plane pick the node" (simple spread
// scheduling): the create request then omits node_id entirely.
export const AUTO_NODE_VALUE = '__auto__'

// Sentinel for "no project", the same reasoning LOCAL_NODE_VALUE
// documents just above; base-ui's Select can't use "" as a real item
// value.
export const NO_PROJECT_VALUE = '__none__'

interface NodeSelectFieldProps {
  idPrefix: string
  nodes: NodeResource[]
  value: string
  onValueChange: (value: string) => void
  error?: Array<{ message?: string } | undefined>
  /** Offers "Automatic" as the first choice (multi-node installs). */
  withAuto?: boolean
}

// Shared by CreateAppFields.tsx and CreateDatabaseFields.tsx: both offer
// the same "which server does this run on" placement picker.
export function NodeSelectField({
  idPrefix,
  nodes,
  value,
  onValueChange,
  error,
  withAuto = false,
}: NodeSelectFieldProps) {
  const { t } = useTranslation('common')
  if (nodes.length === 0) return null
  const id = `${idPrefix}-node`
  const shown =
    !withAuto && value === AUTO_NODE_VALUE ? LOCAL_NODE_VALUE : value
  return (
    <Field>
      <FieldLabel htmlFor={id}>Node</FieldLabel>
      <Select
        value={shown ?? LOCAL_NODE_VALUE}
        onValueChange={(v) => {
          onValueChange(v ?? LOCAL_NODE_VALUE)
        }}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {withAuto ? (
            <SelectItem value={AUTO_NODE_VALUE}>
              {t('nodeSelect.auto')}
            </SelectItem>
          ) : null}
          <SelectItem value={LOCAL_NODE_VALUE}>
            This control plane (local)
          </SelectItem>
          {nodes.map((node) => (
            <SelectItem key={node.id} value={node.id}>
              {node.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <FieldHint>
        {withAuto
          ? t('nodeSelect.autoHint')
          : 'Which server this runs on. Leave it on local unless you\u2019ve added another server to manage.'}
      </FieldHint>
      <FieldError errors={error} />
    </Field>
  )
}

interface ProjectSelectFieldProps {
  idPrefix: string
  projects: ProjectResource[]
  value: string
  onValueChange: (value: string) => void
  error?: Array<{ message?: string } | undefined>
}

// Shared by CreateAppFields.tsx and CreateDatabaseFields.tsx: both offer
// the same optional "group this under a project" picker.
export function ProjectSelectField({
  idPrefix,
  projects,
  value,
  onValueChange,
  error,
}: ProjectSelectFieldProps) {
  if (projects.length === 0) return null
  const id = `${idPrefix}-project`
  return (
    <Field>
      <FieldLabel htmlFor={id}>Project</FieldLabel>
      <Select
        value={value ?? NO_PROJECT_VALUE}
        onValueChange={(v) => {
          onValueChange(v ?? NO_PROJECT_VALUE)
        }}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NO_PROJECT_VALUE}>No project</SelectItem>
          {projects.map((project) => (
            <SelectItem key={project.id} value={project.id}>
              {project.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <FieldHint>
        Optional grouping to keep related apps and databases together.
      </FieldHint>
      <FieldError errors={error} />
    </Field>
  )
}
