import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  DatabaseIcon,
  GitBranchIcon,
  PackageIcon,
  PlusIcon,
  StackIcon,
} from '@phosphor-icons/react/dist/ssr'
import { BrandIcon } from './BrandIcon'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { CreateResourceWizard } from './CreateResourceWizard'

type Quick =
  | { kind: 'start' }
  | {
      kind: 'option'
      value:
        | 'dockerfile-git'
        | 'docker-image'
        | 'docker-compose'
        | 'browse-templates'
    }
  | { kind: 'database' }

/** NewAppMenu is the New app button plus a caret that jumps straight to a deploy source, skipping the picker step. */
export function NewAppMenu() {
  const { t } = useTranslation('dashboard')
  const [quick, setQuick] = useState<Quick | null>(null)
  const items = [
    {
      key: 'git',
      title: t('newApp.items.git.title'),
      hint: t('newApp.items.git.hint'),
      icon: <GitBranchIcon />,
      to: { kind: 'option', value: 'dockerfile-git' } as const,
    },
    {
      key: 'image',
      title: t('newApp.items.image.title'),
      hint: t('newApp.items.image.hint'),
      icon: <BrandIcon name="docker" className="size-4" />,
      to: { kind: 'option', value: 'docker-image' } as const,
    },
    {
      key: 'template',
      title: t('newApp.items.template.title'),
      hint: t('newApp.items.template.hint'),
      icon: <PackageIcon />,
      to: { kind: 'option', value: 'browse-templates' } as const,
    },
    {
      key: 'compose',
      title: t('newApp.items.compose.title'),
      hint: t('newApp.items.compose.hint'),
      icon: <StackIcon />,
      to: { kind: 'option', value: 'docker-compose' } as const,
    },
    {
      key: 'database',
      title: t('newApp.items.database.title'),
      hint: t('newApp.items.database.hint'),
      icon: <DatabaseIcon />,
      to: { kind: 'database' } as const,
    },
  ]
  return (
    <>
      <span className="inline-flex">
        <Button
          size="sm"
          className="rounded-r-none"
          onClick={() => {
            setQuick({ kind: 'start' })
          }}
        >
          <PlusIcon />
          {t('newApp.label')}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                size="sm"
                aria-label={t('newApp.more')}
                className="rounded-l-none border-l border-primary-foreground/20 px-1.5"
              />
            }
          >
            <CaretDownIcon />
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-60">
            {items.map((i) => (
              <DropdownMenuItem
                key={i.key}
                onClick={() => {
                  setQuick(i.to)
                }}
              >
                {i.icon}
                <span className="flex flex-col">
                  <span className="text-foreground">{i.title}</span>
                  <span className="text-xs text-muted-foreground">
                    {i.hint}
                  </span>
                </span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
      {quick ? (
        <CreateResourceWizard
          key={JSON.stringify(quick)}
          open
          onOpenChange={(o) => {
            if (!o) setQuick(null)
          }}
          scope={quick.kind === 'database' ? 'databases' : undefined}
          initialSelected={quick.kind === 'option' ? quick.value : undefined}
        />
      ) : null}
    </>
  )
}
