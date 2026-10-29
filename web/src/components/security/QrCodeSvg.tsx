import { QRCodeSVG } from 'qrcode.react'

// Always dark on white, regardless of theme, so scanners work in dark mode.
export default function QrCodeSvg({
  value,
  label,
  size,
}: {
  value: string
  label: string
  size: number
}) {
  return (
    <QRCodeSVG
      value={value}
      size={size}
      level="M"
      marginSize={0}
      fgColor="#000000"
      bgColor="#ffffff"
      role="img"
      aria-label={label}
      className="block"
    />
  )
}
