import { useState } from 'react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { useRegistryStatus } from '../queries/registry'
import {
  useRegistryRepositoriesOptional,
  useRegistryTagsOptional,
} from '../queries/registryCatalog'

// RegistryImagePicker is the "existing image" build step's repository/tag
// browser: a cascading pair of Selects backed by the built-in registry's
// own catalog (GET /api/v1/registry/repositories, GET
// /api/v1/registry/tags), the same two-level cascading shape
// GitRepoSourcePicker.tsx already establishes for git repo+branch. Picking
// a tag calls onSelect with the full `host/repository:tag` reference; the
// caller (CreateAppFields.tsx, GitBuildSourceFields.tsx) sets that on its
// own image field, exactly as if the operator had typed it by hand.
//
// Renders nothing at all when the built-in registry isn't enabled,
// running, and credentialed yet (or when its own settings lookup hasn't
// resolved), rather than a disabled or empty-looking dropdown: the plain
// text image input the caller renders alongside this is always there and
// always works regardless, so there is nothing broken to paper over, only
// a convenience that isn't available yet.
export function RegistryImagePicker({
  disabled,
  onSelect,
}: {
  disabled?: boolean
  /** Called once a tag is picked, with the full `host/repository:tag`
   *  reference. Picking a different repository or a different tag fires
   *  again with the new value, the same "last pick wins" model
   *  GitRepoSourcePicker's own onSelect documents. */
  onSelect: (imageRef: string) => void
}) {
  const registryStatus = useRegistryStatus()
  const registry = registryStatus.data
  const usable = Boolean(registry?.enabled && registry.status === 'running' && registry.host)

  const [selectedRepo, setSelectedRepo] = useState('')
  const repos = useRegistryRepositoriesOptional(usable)
  const tags = useRegistryTagsOptional(usable && selectedRepo !== '' ? selectedRepo : null)

  if (!usable || !registry?.host) return null
  const host = registry.host

  return (
    <div className="space-y-3 rounded-lg border border-dashed border-border p-3">
      <FieldDescription>
        Browse images already pushed to the built-in registry, or type a full
        reference below instead.
      </FieldDescription>

      <Field>
        <FieldLabel htmlFor="registry-picker-repo">Repository</FieldLabel>
        <Select
          value={selectedRepo}
          onValueChange={(value) => {
            if (typeof value === 'string') setSelectedRepo(value)
          }}
          disabled={disabled || repos.isLoading}
        >
          <SelectTrigger id="registry-picker-repo" className="w-full font-mono">
            <SelectValue
              placeholder={repos.isLoading ? 'Loading repositories...' : 'Select a repository'}
            />
          </SelectTrigger>
          <SelectContent>
            {(repos.data ?? []).map((repo) => (
              <SelectItem key={repo} value={repo} className="font-mono">
                {repo}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {repos.isError ? (
          <p className="text-xs text-destructive">{repos.error.message}</p>
        ) : null}
        {!repos.isLoading && !repos.isError && (repos.data ?? []).length === 0 ? (
          <p className="text-xs text-muted-foreground">
            No images have been pushed to the built-in registry yet.
          </p>
        ) : null}
      </Field>

      {selectedRepo ? (
        <Field>
          <FieldLabel htmlFor="registry-picker-tag">Tag</FieldLabel>
          <Select
            value=""
            onValueChange={(tag) => {
              if (typeof tag !== 'string' || !tag) return
              onSelect(`${host}/${selectedRepo}:${tag}`)
            }}
            disabled={disabled || tags.isLoading}
          >
            <SelectTrigger id="registry-picker-tag" className="w-full font-mono">
              <SelectValue placeholder={tags.isLoading ? 'Loading tags...' : 'Select a tag'} />
            </SelectTrigger>
            <SelectContent>
              {(tags.data ?? []).map((tag) => (
                <SelectItem key={tag} value={tag} className="font-mono">
                  {tag}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {tags.isError ? (
            <p className="text-xs text-destructive">{tags.error.message}</p>
          ) : null}
        </Field>
      ) : null}
    </div>
  )
}
