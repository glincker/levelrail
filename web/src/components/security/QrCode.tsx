import { Suspense, lazy } from 'react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'

// Lazy so the QR encoder stays out of the main bundle; rendered locally,
// the secret never leaves the browser.
const QrCodeSvg = lazy(() => import('./QrCodeSvg'))

const QR_SIZE = 160

export function QrCode({ value }: { value: string }) {
  const { t } = useTranslation('settings')
  return (
    <div className="flex justify-center">
      <div className="size-[184px] rounded-lg bg-white p-3">
        <Suspense
          fallback={
            <Skeleton
              className="size-[160px]"
              role="img"
              aria-label={t('accountPage.twoFactor.qrLoading')}
            />
          }
        >
          <QrCodeSvg
            value={value}
            size={QR_SIZE}
            label={t('accountPage.twoFactor.qrLabel')}
          />
        </Suspense>
      </div>
    </div>
  )
}
