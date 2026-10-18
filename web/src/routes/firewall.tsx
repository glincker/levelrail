import { createFileRoute, Outlet } from '@tanstack/react-router'
import { validateFirewallSearch } from '../components/firewall/useFirewallNav'

export const Route = createFileRoute('/firewall')({
  validateSearch: validateFirewallSearch,
  component: Outlet,
})
