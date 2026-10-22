import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function AppsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('common')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '90 90 370 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4">
        <path d="M130 136L200 171" className="stroke-(--iso-accent-l)" />
        <path d="M290 185L380 140" className="stroke-(--iso-l)" />
      </g>
      <Block x={130} y={120} a={26} h={12} p={YELLOW} />
      <g
        className="stroke-(--iso-accent-r)"
        strokeWidth="1.3"
        strokeLinecap="round"
      >
        <path d="M126 125V113" />
        <path d="M126 119C126 115 134 117 134 113" />
        <circle cx="126" cy="125" r="1.8" className="fill-card" />
        <circle cx="126" cy="113" r="1.8" className="fill-card" />
        <circle cx="134" cy="113" r="1.8" className="fill-card" />
      </g>
      <Block x={250} y={185} a={40} h={28} p={BLUE} />
      <Block x={410} y={130} a={28} h={14} p={GREY} />
      <rect
        x="401"
        y="125"
        width="18"
        height="11"
        rx="1.5"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      <path
        d="M401 128H419"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M130 106V84" />
            <path d="M250 163V130" />
            <path d="M410 116V94" />
          </g>
          <g className={LABEL} textAnchor="middle">
            <text x="130" y="76">
              {t('appsList.banner.diagram.git')}
            </text>
            <text x="250" y="122">
              {t('appsList.banner.diagram.app')}
            </text>
            <text x="410" y="86">
              {t('appsList.banner.diagram.browser')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
