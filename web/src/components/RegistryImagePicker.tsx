import { useState } from 'react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
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
import {
  splitDockerHubRepoName,
  useDockerHubSearch,
  useDockerHubTags,
  type DockerHubRepository,
} from '../queries/dockerhub'
import { useDebouncedValue } from '../hooks/useDebouncedValue'

const BUILTIN_SOURCE_ID = 'builtin'
const DOCKERHUB_SOURCE_ID = 'dockerhub'
const DOCKERHUB_SEARCH_DEBOUNCE_MS = 350

function buildImageRef(host: string, repository: string, tag: string): string {
  return `${host}/${repository}:${tag}`
}

// RegistryImagePicker is the "existing image" build step's repository/tag
// browser: a source selector (once there is more than one source) over
// either a cascading pair of Selects (built-in registry, a connected
// registry credential) or a debounced public Docker Hub search. Backed by
// the built-in registry's own catalog (GET /api/v1/registry/repositories,
// GET /api/v1/registry/tags), an external credential's own catalog (GET
// /api/v1/registry-credentials/{id}/repositories, GET .../tags), or
// public Docker Hub search (GET /api/v1/dockerhub/search, GET
// /api/v1/dockerhub/repositories/{namespace}/{repo}/tags) for a
// well-known public image that isn't in either. Picking a tag calls
// onSelect with the full `host/repository:tag` reference; the caller
// (CreateAppFields.tsx, GitBuildSourceFields.tsx) sets that on its own
// image field, exactly as if the operator had typed it by hand.
//
// Docker Hub search needs no configuration, so it is always one of the
// sources; the built-in registry and any connected credential are added
// ahead of it when usable. The source selector itself only appears once
// there is an actual choice to make (more than one source), the same
// "don't show a picker with nothing to pick" reasoning the old
// credentials-only gate used.
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
  if (builtinAvailable)
    sources.push({ id: BUILTIN_SOURCE_ID, label: 'Built-in registry' })
  for (const credential of credentials) {
    sources.push({
      id: credential.id,
      label: `${credential.name} (${credential.registry_host})`,
    })
  }
  sources.push({ id: DOCKERHUB_SOURCE_ID, label: 'Docker Hub' })

  const [source, setSource] = useState('')
  const [selectedRepo, setSelectedRepo] = useState('')
  const [dockerHubQuery, setDockerHubQuery] = useState('')
  const [selectedDockerHubRepo, setSelectedDockerHubRepo] = useState('')
  const effectiveSource = sources.some((option) => option.id === source)
    ? source
    : (sources[0]?.id ?? '')

  const isBuiltinSelected = effectiveSource === BUILTIN_SOURCE_ID
  const isDockerHubSelected = effectiveSource === DOCKERHUB_SOURCE_ID
  const selectedCredential =
    isBuiltinSelected || isDockerHubSelected
      ? null
      : (credentials.find((credential) => credential.id === effectiveSource) ??
        null)

  const builtinRepos = useRegistryRepositoriesOptional(
    builtinAvailable && isBuiltinSelected,
  )
  const builtinTags = useRegistryTagsOptional(
    builtinAvailable && isBuiltinSelected && selectedRepo !== ''
      ? selectedRepo
      : null,
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

  const debouncedDockerHubQuery = useDebouncedValue(
    isDockerHubSelected ? dockerHubQuery : '',
    DOCKERHUB_SEARCH_DEBOUNCE_MS,
  )
  const dockerHubSearch = useDockerHubSearch(debouncedDockerHubQuery)
  const dockerHubRepoParts = selectedDockerHubRepo
    ? splitDockerHubRepoName(selectedDockerHubRepo)
    : null
  const dockerHubTags = useDockerHubTags(
    dockerHubRepoParts?.namespace ?? '',
    dockerHubRepoParts ? dockerHubRepoParts.repo : null,
  )

  const repos = isBuiltinSelected ? builtinRepos : credentialRepos
  const tags = isBuiltinSelected ? builtinTags : credentialTags
  const host = isBuiltinSelected
    ? registry?.host
    : selectedCredential?.registry_host

  function handleSourceChange(next: string) {
    setSource(next)
    setSelectedRepo('')
    setDockerHubQuery('')
    setSelectedDockerHubRepo('')
  }

  return (
    <div className="space-y-3 rounded-lg border border-dashed border-border p-3">
      <FieldDescription>
        {hasCredentials
          ? 'Browse images already pushed to a registry, search public Docker Hub, or type a full reference below instead.'
          : 'Browse images already pushed to the built-in registry, search public Docker Hub, or type a full reference below instead.'}
      </FieldDescription>

      {sources.length > 1 ? (
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

      {isDockerHubSelected ? (
        <DockerHubSearchFields
          disabled={disabled}
          query={dockerHubQuery}
          onQueryChange={(value) => {
            setDockerHubQuery(value)
            setSelectedDockerHubRepo('')
          }}
          results={dockerHubSearch.data ?? []}
          isLoading={dockerHubSearch.isLoading}
          error={dockerHubSearch.isError ? dockerHubSearch.error.message : null}
          selectedRepo={selectedDockerHubRepo}
          onSelectRepo={setSelectedDockerHubRepo}
          tags={(dockerHubTags.data ?? []).map((tag) => tag.name)}
          tagsLoading={dockerHubTags.isLoading}
          tagsError={dockerHubTags.isError ? dockerHubTags.error.message : null}
          onSelectTag={(tag) =>
            onSelect(buildImageRef('docker.io', selectedDockerHubRepo, tag))
          }
        />
      ) : (
        <>
          <Field>
            <FieldLabel htmlFor="registry-picker-repo">Repository</FieldLabel>
            <Select
              value={selectedRepo}
              onValueChange={(value) => {
                if (typeof value === 'string') setSelectedRepo(value)
              }}
              disabled={disabled || repos.isLoading}
            >
              <SelectTrigger
                id="registry-picker-repo"
                className="w-full font-mono"
              >
                <SelectValue
                  placeholder={
                    repos.isLoading
                      ? 'Loading repositories...'
                      : 'Select a repository'
                  }
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
            {!repos.isLoading &&
            !repos.isError &&
            (repos.data ?? []).length === 0 ? (
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
                <SelectTrigger
                  id="registry-picker-tag"
                  className="w-full font-mono"
                >
                  <SelectValue
                    placeholder={
                      tags.isLoading ? 'Loading tags...' : 'Select a tag'
                    }
                  />
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
        </>
      )}
    </div>
  )
}

// DockerHubSearchFields is the Docker Hub source's own UI: a debounced
// search input over a results list (instead of the other sources'
// cascading repository Select, since Docker Hub has no fixed catalog to
// enumerate), then the same tag-select shape once a result is picked.
// Split out purely to keep RegistryImagePicker's own render function
// readable, no other meaning to the split.
function DockerHubSearchFields({
  disabled,
  query,
  onQueryChange,
  results,
  isLoading,
  error,
  selectedRepo,
  onSelectRepo,
  tags,
  tagsLoading,
  tagsError,
  onSelectTag,
}: {
  disabled?: boolean
  query: string
  onQueryChange: (value: string) => void
  results: DockerHubRepository[]
  isLoading: boolean
  error: string | null
  selectedRepo: string
  onSelectRepo: (repoName: string) => void
  tags: string[]
  tagsLoading: boolean
  tagsError: string | null
  onSelectTag: (tag: string) => void
}) {
  const hasQuery = query.trim() !== ''

  return (
    <>
      <Field>
        <FieldLabel htmlFor="dockerhub-search">Search Docker Hub</FieldLabel>
        <Input
          id="dockerhub-search"
          placeholder="e.g. postgres"
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          disabled={disabled}
        />
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
      </Field>

      {hasQuery ? (
        <div
          role="listbox"
          aria-label="Docker Hub search results"
          className="max-h-56 space-y-1 overflow-y-auto rounded-md border border-border p-1"
        >
          {isLoading ? (
            <p className="p-2 text-xs text-muted-foreground">
              Searching Docker Hub...
            </p>
          ) : results.length === 0 ? (
            <p className="p-2 text-xs text-muted-foreground">
              No matching public images found.
            </p>
          ) : (
            results.map((repo) => (
              <button
                key={repo.repo_name}
                type="button"
                disabled={disabled}
                onClick={() => onSelectRepo(repo.repo_name)}
                aria-selected={selectedRepo === repo.repo_name}
                className={cn(
                  'flex w-full flex-col gap-0.5 rounded-md px-2 py-1.5 text-left hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50',
                  selectedRepo === repo.repo_name && 'bg-accent',
                )}
              >
                <span className="flex items-center gap-2">
                  <span className="font-mono text-sm">{repo.repo_name}</span>
                  {repo.is_official ? (
                    <Badge variant="success">Official</Badge>
                  ) : null}
                  {repo.is_automated ? (
                    <Badge variant="outline">Automated</Badge>
                  ) : null}
                </span>
                {repo.short_description ? (
                  <span className="truncate text-xs text-muted-foreground">
                    {repo.short_description}
                  </span>
                ) : null}
                <span className="text-xs text-muted-foreground">
                  {repo.star_count.toLocaleString()} stars
                </span>
              </button>
            ))
          )}
        </div>
      ) : null}

      {selectedRepo ? (
        <Field>
          <FieldLabel htmlFor="dockerhub-tag">Tag</FieldLabel>
          <Select
            value=""
            onValueChange={(tag) => {
              if (typeof tag !== 'string' || !tag) return
              onSelectTag(tag)
            }}
            disabled={disabled || tagsLoading}
          >
            <SelectTrigger id="dockerhub-tag" className="w-full font-mono">
              <SelectValue
                placeholder={tagsLoading ? 'Loading tags...' : 'Select a tag'}
              />
            </SelectTrigger>
            <SelectContent>
              {tags.map((tag) => (
                <SelectItem key={tag} value={tag} className="font-mono">
                  {tag}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {tagsError ? (
            <p className="text-xs text-destructive">{tagsError}</p>
          ) : null}
        </Field>
      ) : null}
    </>
  )
}
