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
import {
  useRegistryCredentialRepositories,
  useRegistryCredentialsOptional,
  useRegistryCredentialTags,
} from '../queries/registryCredentials'

const BUILTIN_SOURCE_ID = 'builtin'

function buildImageRef(host: string, repository: string, tag: string): string {
  return `${host}/${repository}:${tag}`
}

// RegistryImagePicker is the "existing image" build step's repository/tag
// browser: a cascading pair of Selects, optionally preceded by a registry
// source selector once the operator has connected external registry
// credentials (Settings -> Registry Credentials). Backed by the built-in
// registry's own catalog (GET /api/v1/registry/repositories, GET
// /api/v1/registry/tags) or, for an external credential, that credential's
// own catalog (GET /api/v1/registry-credentials/{id}/repositories, GET
// .../tags), the same two-level cascading shape either way. Picking a tag
// calls onSelect with the full `host/repository:tag` reference, host
// coming from whichever registry is selected; the caller
// (CreateAppFields.tsx, GitBuildSourceFields.tsx) sets that on its own
// image field, exactly as if the operator had typed it by hand.
//
// Renders nothing when neither the built-in registry is usable (enabled,
// running, credentialed) nor any registry credential is connected: the
// plain text image input the caller renders alongside this is always
// there and always works regardless, so there is nothing broken to paper
// over, only a convenience that isn't available yet. The source selector
// itself only appears once there is an actual choice to make: zero
// connected credentials falls straight through to the built-in-only
// cascade, unchanged.
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
  const builtinAvailable = Boolean(
    registry?.enabled && registry.status === 'running' && registry.host,
  )

  const credentialsQuery = useRegistryCredentialsOptional()
  const credentials = credentialsQuery.data ?? []
  const hasCredentials = credentials.length > 0

  const sources: { id: string; label: string }[] = []
  if (builtinAvailable) sources.push({ id: BUILTIN_SOURCE_ID, label: 'Built-in registry' })
  for (const credential of credentials) {
    sources.push({
      id: credential.id,
      label: `${credential.name} (${credential.registry_host})`,
    })
  }

  const [source, setSource] = useState('')
  const [selectedRepo, setSelectedRepo] = useState('')
  const effectiveSource = sources.some((option) => option.id === source)
    ? source
    : (sources[0]?.id ?? '')

  const isBuiltinSelected = effectiveSource === BUILTIN_SOURCE_ID
  const selectedCredential = isBuiltinSelected
    ? null
    : (credentials.find((credential) => credential.id === effectiveSource) ?? null)

  const builtinRepos = useRegistryRepositoriesOptional(builtinAvailable && isBuiltinSelected)
  const builtinTags = useRegistryTagsOptional(
    builtinAvailable && isBuiltinSelected && selectedRepo !== '' ? selectedRepo : null,
  )
  const credentialRepos = useRegistryCredentialRepositories(
    selectedCredential?.id ?? '',
    selectedCredential !== null,
  )
  const credentialTags = useRegistryCredentialTags(
    selectedCredential?.id ?? '',
    selectedRepo !== '' ? selectedRepo : null,
    selectedCredential !== null,
  )

  if (sources.length === 0) return null

  const repos = isBuiltinSelected ? builtinRepos : credentialRepos
  const tags = isBuiltinSelected ? builtinTags : credentialTags
  const host = isBuiltinSelected ? registry?.host : selectedCredential?.registry_host

  function handleSourceChange(next: string) {
    setSource(next)
    setSelectedRepo('')
  }

  return (
    <div className="space-y-3 rounded-lg border border-dashed border-border p-3">
      <FieldDescription>
        {hasCredentials
          ? 'Browse images already pushed to a registry, or type a full reference below instead.'
          : 'Browse images already pushed to the built-in registry, or type a full reference below instead.'}
      </FieldDescription>

      {hasCredentials ? (
        <Field>
          <FieldLabel htmlFor="registry-picker-source">Registry</FieldLabel>
          <Select
            value={effectiveSource}
            onValueChange={(value) => {
              if (typeof value === 'string') handleSourceChange(value)
            }}
            disabled={disabled}
          >
            <SelectTrigger id="registry-picker-source" className="w-full">
              <SelectValue placeholder="Select a registry" />
            </SelectTrigger>
            <SelectContent>
              {sources.map((option) => (
                <SelectItem key={option.id} value={option.id}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      ) : null}

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
            {isBuiltinSelected
              ? 'No images have been pushed to the built-in registry yet.'
              : 'No repositories found in this registry yet.'}
          </p>
        ) : null}
      </Field>

      {selectedRepo ? (
        <Field>
          <FieldLabel htmlFor="registry-picker-tag">Tag</FieldLabel>
          <Select
            value=""
            onValueChange={(tag) => {
              if (typeof tag !== 'string' || !tag || !host) return
              onSelect(buildImageRef(host, selectedRepo, tag))
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
