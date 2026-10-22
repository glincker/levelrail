import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function HealthDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('settings')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '110 80 350 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4" className="stroke-(--iso-l)">
        <path d="M318 150L350 106" />
        <path d="M318 150L398 160" />
        <path d="M318 150L336 192" />
      </g>
      {[0, 1, 2].map((i) => (
        <g key={i}>
          <Block x={150} y={150 - i * 16} a={40} h={11} p={BLUE} />
          <circle cx={121} cy={153 - i * 16} r="1.8" className="fill-card" />
        </g>
      ))}
      <path
        d="M192 150H212L222 130L234 168L244 140L252 150H288"
        className="stroke-(--iso-accent-top)"
        strokeWidth="2"
        strokeLinejoin="round"
      />
      <Block x={300} y={150} a={9} h={9} p={YELLOW} />
      <Block x={370} y={100} a={24} h={12} p={GREY} />
      <Block x={420} y={160} a={24} h={12} p={GREY} />
      <Block x={340} y={195} a={24} h={12} p={GREY} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M370 74V86" />
            <path d="M446 160H452" />
            <path d="M340 223V233" />
          </g>
          <g className={LABEL}>
            <text x="370" y="70" textAnchor="middle">
              {t('controlPlaneHealth.banner.diagram.database')}
            </text>
            <text x="456" y="165">
              {t('controlPlaneHealth.banner.diagram.disk')}
            </text>
            <text x="340" y="248" textAnchor="middle">
              {t('controlPlaneHealth.banner.diagram.certificates')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
