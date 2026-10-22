import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, LABEL, YELLOW } from './isoTokens'

export default function DeploymentsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('deploys')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '50 100 380 160'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4">
        <path d="M150 156L266 150" className="stroke-(--iso-l)" />
        <path
          d="M300 214Q220 262 140 218"
          className="stroke-(--iso-accent-l)"
        />
      </g>
      <path
        d="M150 208L138 218L152 224"
        className="stroke-(--iso-accent-l)"
        strokeWidth="1.5"
      />
      <path
        d="M258 144L267 150L258 156"
        className="stroke-(--iso-l)"
        strokeWidth="1.5"
      />
      <Block x={110} y={150} a={30} h={26} p={YELLOW} />
      <Block x={330} y={150} a={60} h={22} p={BLUE} />
      <g className="fill-card">
        <circle cx="305" cy="173" r="2.2" />
        <circle cx="315" cy="178" r="2.2" />
      </g>
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M110 120V104" />
            <path d="M330 118V104" />
          </g>
          <g className={LABEL} textAnchor="middle">
            <text x="110" y="96">
              {t('banner.diagram.image')}
            </text>
            <text x="330" y="96">
              {t('banner.diagram.server')}
            </text>
            <text x="220" y="272" className="fill-foreground">
              {t('banner.diagram.rollback')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
