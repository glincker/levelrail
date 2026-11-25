import { DeviceLoginBanner } from '../attention/DeviceLoginBanner'
import { SignInRequestsBanner } from '../attention/SignInRequestsBanner'
import { InstallCard } from '../InstallCard'
import { LocalModeBanner } from '../LocalModeBanner'
import { RestartBanner } from '../RestartBanner'
import { DomainMaintenanceBanner } from '../maintenance/DomainMaintenanceBanner'
import { MaintenanceBanner } from '../maintenance/MaintenanceBanner'
import { UpgradeGlobalBar } from '../settings/updates/UpgradeGlobalBar'
import { UpgradeOutcomeToast } from '../settings/updates/UpgradeOutcomeToast'
import { VacationBanner } from './VacationBanner'

export default function ShellBanners() {
  return (
    <>
      <VacationBanner />
      <RestartBanner />
      <LocalModeBanner />
      <UpgradeGlobalBar />
      <UpgradeOutcomeToast />
      <MaintenanceBanner />
      <DomainMaintenanceBanner />
      <DeviceLoginBanner />
      <SignInRequestsBanner />
      <InstallCard />
    </>
  )
}
