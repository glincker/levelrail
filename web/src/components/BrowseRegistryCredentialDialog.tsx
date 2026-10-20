import { useState } from 'react'
import { FolderOpenIcon, TagIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import {
  useRegistryCredentialRepositories,
  useRegistryCredentialTags,
} from '../queries/registryCredentials'
import type { RegistryCredential } from '../types/registryCredential'

// Browses a stored external registry credential's own catalog: pick a
// repository, then see its tags, the same two-level drill-down
// RegistryImagePicker.tsx uses for the built-in registry's own catalog.
// Read-only and not wired to any form: this is a lookup aid for finding
// a value to type into a direct-image app's image field elsewhere, not
// itself a picker.
export function BrowseRegistryCredentialDialog({
  credential,
}: {
  credential: RegistryCredential
}) {
  const [open, setOpen] = useState(false)
  const [selectedRepo, setSelectedRepo] = useState<string | null>(null)

  const repos = useRegistryCredentialRepositories(credential.id, open)
  const tags = useRegistryCredentialTags(credential.id, selectedRepo, open)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) setSelectedRepo(null)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <FolderOpenIcon className="size-3.5" aria-hidden="true" />
        Browse
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Browse &ldquo;{credential.name}&rdquo;</DialogTitle>
          <DialogDescription>
            Repositories and tags visible to this credential in{' '}
            <span className="font-mono">{credential.registry_host}</span>.
          </DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">
              Repositories
            </p>
            {repos.isLoading ? (
              <p className="text-sm text-muted-foreground">Loading...</p>
            ) : repos.isError ? (
              <p className="text-sm text-destructive">{repos.error.message}</p>
            ) : (repos.data ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No repositories found.
              </p>
            ) : (
              <ul className="max-h-64 space-y-0.5 overflow-y-auto">
                {(repos.data ?? []).map((repo) => (
                  <li key={repo}>
                    <button
                      type="button"
                      onClick={() => setSelectedRepo(repo)}
                      className={`w-full truncate rounded px-2 py-1 text-left font-mono text-sm hover:bg-muted ${
                        selectedRepo === repo
                          ? 'bg-muted text-foreground'
                          : 'text-muted-foreground'
                      }`}
                    >
                      {repo}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">Tags</p>
            {!selectedRepo ? (
              <p className="text-sm text-muted-foreground">
                Select a repository.
              </p>
            ) : tags.isLoading ? (
              <p className="text-sm text-muted-foreground">Loading...</p>
            ) : tags.isError ? (
              <p className="text-sm text-destructive">{tags.error.message}</p>
            ) : (tags.data ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">No tags found.</p>
            ) : (
              <ul className="max-h-64 space-y-0.5 overflow-y-auto">
                {(tags.data ?? []).map((tag) => (
                  <li
                    key={tag}
                    className="flex items-center gap-1.5 truncate rounded px-2 py-1 font-mono text-sm text-muted-foreground"
                  >
                    <TagIcon className="size-3 shrink-0" aria-hidden="true" />
                    {tag}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
