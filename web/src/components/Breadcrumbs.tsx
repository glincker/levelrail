import { Link } from '@tanstack/react-router'
import { CaretRightIcon, HouseIcon } from '@phosphor-icons/react/dist/ssr'
import { useProjectListOptional } from '../queries/projects'
import { useOrganizationListOptional } from '../queries/organizations'
import { useEnvironmentListOptional } from '../queries/environments'

interface BreadcrumbsProps {
  // Set directly on the organization detail route itself, where there is
  // no project in context to derive it from. Every other call site
  // leaves this unset and lets it resolve from project.org_id below.
  organizationId?: string
  projectId?: string
  environmentId?: string
  appName?: string
  page?: string
}

const CRUMB_LINK_CLASS = 'text-muted-foreground hover:text-foreground'
const CRUMB_CURRENT_CLASS = 'font-medium text-foreground'

function Separator() {
  return (
    <CaretRightIcon
      className="size-3 shrink-0 text-muted-foreground/50"
      aria-hidden="true"
    />
  )
}

// Renders Home > Organization > Project > Environment > App > page,
// skipping any segment whose id is absent (an app with no project set is
// the common case, not an edge case). Names come from the same optional,
// non-suspense list queries AppOverview.tsx already resolves project_id/
// environment_id through: a slow or failed name lookup just means that
// segment doesn't render yet, or falls back to the raw id, never a
// crash or a link built from data that isn't there.
export function Breadcrumbs({
  organizationId,
  projectId,
  environmentId,
  appName,
  page,
}: BreadcrumbsProps) {
  const projectList = useProjectListOptional()
  const project = projectId
    ? projectList.data?.find((p) => p.id === projectId)
    : undefined
  const resolvedOrgId = organizationId ?? project?.org_id
  const organizationList = useOrganizationListOptional()
  const organization = resolvedOrgId
    ? organizationList.data?.find((o) => o.id === resolvedOrgId)
    : undefined
  const environmentList = useEnvironmentListOptional(projectId ?? '')
  const environment = environmentId
    ? environmentList.data?.find((e) => e.id === environmentId)
    : undefined

  const hasAfterOrg = Boolean(projectId)
  const hasAfterProject =
    Boolean(environmentId) || Boolean(appName) || Boolean(page)
  const hasAfterEnvironment = Boolean(appName) || Boolean(page)
  const hasAfterApp = Boolean(page)

  return (
    <nav
      aria-label="Breadcrumb"
      className="flex flex-wrap items-center gap-1.5 text-xs"
    >
      <Link to="/" className={CRUMB_LINK_CLASS} aria-label="Home">
        <HouseIcon className="size-3.5" aria-hidden="true" />
      </Link>

      {resolvedOrgId ? (
        <>
          <Separator />
          {hasAfterOrg ? (
            <Link
              to="/organizations/$id"
              params={{ id: resolvedOrgId }}
              className={CRUMB_LINK_CLASS}
            >
              {organization?.name ?? resolvedOrgId}
            </Link>
          ) : (
            <span className={CRUMB_CURRENT_CLASS}>
              {organization?.name ?? resolvedOrgId}
            </span>
          )}
        </>
      ) : null}

      {projectId ? (
        <>
          <Separator />
          {hasAfterProject ? (
            <Link
              to="/projects/$id"
              params={{ id: projectId }}
              className={CRUMB_LINK_CLASS}
            >
              {project?.name ?? projectId}
            </Link>
          ) : (
            <span className={CRUMB_CURRENT_CLASS}>
              {project?.name ?? projectId}
            </span>
          )}
        </>
      ) : null}

      {environmentId ? (
        <>
          <Separator />
          {projectId ? (
            hasAfterEnvironment ? (
              <Link
                to="/projects/$id/environments/$envId"
                params={{ id: projectId, envId: environmentId }}
                className={CRUMB_LINK_CLASS}
              >
                {environment?.name ?? environmentId}
              </Link>
            ) : (
              <span className={CRUMB_CURRENT_CLASS}>
                {environment?.name ?? environmentId}
              </span>
            )
          ) : (
            // No project id to build /projects/$id/environments/$envId
            // from: render the label without a link rather than guess a
            // broken path.
            <span className="italic text-muted-foreground">
              {environment?.name ?? environmentId}
            </span>
          )}
        </>
      ) : null}

      {appName ? (
        <>
          <Separator />
          {hasAfterApp ? (
            <Link
              to="/apps/$name/overview"
              params={{ name: appName }}
              className={CRUMB_LINK_CLASS}
            >
              {appName}
            </Link>
          ) : (
            <span className={CRUMB_CURRENT_CLASS}>{appName}</span>
          )}
        </>
      ) : null}

      {page ? (
        <>
          <Separator />
          <span className={CRUMB_CURRENT_CLASS}>{page}</span>
        </>
      ) : null}
    </nav>
  )
}
