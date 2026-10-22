import { Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { BrandIcon } from '../BrandIcon'
import { SetPrimaryDomainPrompt } from '../SetPrimaryDomainPrompt'
import { ingressSettingsQueryOptions } from '../../queries/domains'
import {
  GIT_HOSTS,
  gitSettingsPath,
  hostNeedsPublicDomain,
} from '../../lib/gitAccounts'
import type { GitProviderName } from '../../types/gitProviders'

const HOST_BRAND = {
  github: 'github',
  gitlab: 'gitlab',
  bitbucket: 'bitbucket',
  bitbucket_server: 'bitbucket',
  gitea: 'gitea',
  forgejo: 'forgejo',
} as const

export function AddAccountMenu() {
  const { t } = useTranslation('settings')
  const navigate = useNavigate()
  const ingress = useQuery(ingressSettingsQueryOptions())
  const [pending, setPending] = useState<GitProviderName | null>(null)
  const hasDomain = Boolean(ingress.data?.primary_domain)

  function go(provider: GitProviderName) {
    void navigate({ to: gitSettingsPath(provider) })
  }

  function choose(provider: GitProviderName) {
    if (hostNeedsPublicDomain(provider) && ingress.isSuccess && !hasDomain) {
      setPending(provider)
      return
    }
    go(provider)
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button type="button" />}>
          <PlusIcon className="size-4" aria-hidden="true" />
          {t('gitConnectionsHub.add.action')}
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-80">
          {GIT_HOSTS.map((provider) => (
            <DropdownMenuItem
              key={provider}
              onClick={() => choose(provider)}
              className="items-start"
            >
              <BrandIcon
                name={HOST_BRAND[provider]}
                className="mt-0.5 size-5"
              />
              <span className="min-w-0">
                <span className="block font-medium text-foreground">
                  {t(`gitConnectionsHub.hosts.${provider}.name`)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(`gitConnectionsHub.hosts.${provider}.asks`)}
                </span>
              </span>
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      <Dialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('gitConnectionsHub.add.domainTitle')}</DialogTitle>
            <DialogDescription>
              {t('gitConnectionsHub.add.domainBody')}
            </DialogDescription>
          </DialogHeader>
          <Suspense fallback={null}>
            <SetPrimaryDomainPrompt />
          </Suspense>
          <DialogFooter>
            <Button
              type="button"
              disabled={!hasDomain || pending === null}
              onClick={() => {
                if (pending) go(pending)
              }}
            >
              {t('gitConnectionsHub.add.continue')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
