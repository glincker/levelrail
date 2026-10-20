import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { HelpLink } from '@/components/HelpLink'
import type { DoctorCheck } from '../queries/systemDoctor'

export interface CheckCta {
  message: string
  action: ReactNode
}

const inlineLinkClassName =
  'inline-flex items-center gap-1 text-sm text-primary underline underline-offset-4 hover:no-underline'

// PORT_CHECK_CODE matches port_<n> for whatever ports this instance's
// ingress is actually configured on (APP_INGRESS_HTTP_ADDR/
// APP_INGRESS_HTTPS_ADDR), not just the literal port_80/port_443 codes
// a default-port instance reports.
const PORT_CHECK_CODE = /^port_\d+$/
// REGISTRY_REACHABILITY_CODE matches registry_reachability_<host>, the
// same "handle generically for any configured host" shape PORT_CHECK_CODE
// already establishes for port_<n>.
const REGISTRY_REACHABILITY_CODE = /^registry_reachability_/

// One CTA per failure mode that actually has a useful next step, keyed by
// doctorCheckResource.code (internal/api/doctor.go) and status. Checks
// with no genuinely actionable next step beyond "fix your host" (the
// database ping, a port check that only failed because it needs elevated
// privileges to probe) are deliberately left without a CTA rather than
// padded out with a link to nothing.
export function getCheckCta(check: DoctorCheck): CheckCta | null {
  // Handle port checks generically for any configured port, not just 80/443
  if (PORT_CHECK_CODE.test(check.code)) {
    if (check.status === 'fail') {
      return {
        message:
          'Something else on this host already has this port bound, commonly another web server (nginx, Apache, a previous Caddy) or a leftover process.',
        action: (
          <HelpLink
            path="/troubleshooting"
            label="Diagnose the port conflict"
            variant="inline"
          />
        ),
      }
    }
    return null
  }

  // Handle registry checks generically for any configured host, not just
  // the Docker Hub default.
  if (REGISTRY_REACHABILITY_CODE.test(check.code)) {
    if (check.status === 'warn') {
      return {
        message:
          'Builds and deploys that pull or push images against this registry will fail until this host can reach it over HTTPS.',
        action: (
          <HelpLink
            path="/troubleshooting"
            label="Diagnose registry reachability"
            variant="inline"
          />
        ),
      }
    }
    return null
  }

  switch (check.code) {
    case 'agent_advertise_reachability':
      if (check.status === 'fail' || check.status === 'warn') {
        return {
          message:
            'A remote agent dials this address to reach the control plane. If it is wrong or unreachable, every node enrolled after the mistake fails to connect with a confusing TLS error.',
          action: (
            <HelpLink
              path="/troubleshooting"
              label="Fix the agent advertise host"
              variant="inline"
            />
          ),
        }
      }
      return null

    case 'disk_io_latency':
      if (check.status === 'warn') {
        return {
          message:
            'Free disk space alone does not catch this: a disk can have plenty of room left and still write slowly enough to make deploys and log writes feel stuck.',
          action: (
            <HelpLink
              path="/troubleshooting"
              label="Diagnose slow disk writes"
              variant="inline"
            />
          ),
        }
      }
      return null

    case 'docker':
      if (check.status === 'fail') {
        return {
          message:
            "The control plane can't reach the Docker daemon. This is almost always a socket permission problem on this host, not a Docker install issue.",
          action: (
            <HelpLink
              path="/troubleshooting"
              label="Fix Docker socket access"
              variant="inline"
            />
          ),
        }
      }
      return null

    case 'disk_space':
      if (check.status === 'warn') {
        return {
          message:
            'Unused Docker images, stopped containers, and build cache can often be cleaned up to reclaim space.',
          action: (
            <Link to="/settings/general" className={inlineLinkClassName}>
              Clean up Docker storage
            </Link>
          ),
        }
      }
      return null

    case 'data_dir_writable':
      if (check.status === 'fail') {
        return {
          message:
            "The data directory isn't writable by the user running this process, usually a permissions or ownership mismatch after a restore or a manual chown.",
          action: (
            <HelpLink
              path="/troubleshooting"
              label="Fix data directory permissions"
              variant="inline"
            />
          ),
        }
      }
      return null

    case 'master_key_rotation':
      if (check.status === 'warn') {
        return {
          message:
            'Rotating regularly limits how long a single exposed key stays useful.',
          action: (
            <HelpLink
              path="/master-key-rotation#how-to-rotate"
              label="How to rotate the master key"
              variant="inline"
            />
          ),
        }
      }
      return null

    case 'secret_binding':
      if (check.status === 'warn') {
        return {
          message:
            'Some secret values predate slot binding. Bind them from General settings, or run the rebind command.',
          action: (
            <HelpLink
              path="/master-key-rotation#binding-secrets-to-their-slot"
              label="Binding secrets to their slot"
              variant="inline"
            />
          ),
        }
      }
      return null

    case 'stale_secrets':
      if (check.status === 'warn') {
        return {
          message:
            'One or more secrets (per-app or shared) have not been rotated in a while. Each app and each project/organization/environment has its own Secrets card to set a fresh value.',
          action: (
            <Link to="/projects" className={inlineLinkClassName}>
              Browse projects and apps
            </Link>
          ),
        }
      }
      return null

    case 'container_runtime':
      if (check.status === 'warn') {
        return {
          message:
            'Rootless Docker and Podman support is best-effort: verify bind-mount ownership and UID mapping for your workload before relying on it in production.',
          action: (
            <HelpLink
              path="/security#rootless-and-podman"
              label="Rootless and Podman support"
              variant="inline"
            />
          ),
        }
      }
      return null

    case 'firewall':
      if (check.status === 'warn' || check.status === 'unknown') {
        return {
          message:
            check.status === 'warn'
              ? "This host's firewall isn't fully locked down against the supported setup."
              : 'Levelrail could not determine this host firewall status. If you rely on a different firewall (cloud security groups, firewalld), this is expected.',
          action: (
            <HelpLink
              path="/security#fresh-box-hardening-checklist"
              label="Firewall setup guide"
              variant="inline"
            />
          ),
        }
      }
      return null

    default:
      return null
  }
}
