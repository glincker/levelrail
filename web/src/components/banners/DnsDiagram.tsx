import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function DnsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('dns')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '70 55 350 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <path
        d="M196 152L336 164"
        className="stroke-(--iso-l)"
        strokeWidth="1.5"
        strokeDasharray="4 4"
      />
      <Block x={150} y={130} a={44} h={10} p={GREY} />
      <g className="stroke-(--iso-sleep-r)" strokeWidth="1.5">
        <path d="M130 124L152 135" />
        <path d="M138 120L160 131" />
        <path d="M122 129L144 140" />
      </g>
      <Block x={370} y={150} a={40} h={12} p={BLUE} />
      <Block x={370} y={134} a={40} h={12} p={BLUE} />
      <circle cx={341} cy={137} r="1.8" className="fill-card" />
      <circle cx={341} cy={153} r="1.8" className="fill-card" />
      <Block x={266} y={158} a={9} h={9} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M150 104V96" />
            <path d="M370 106V98" />
          </g>
          <g className={LABEL}>
            <text x="150" y="88" textAnchor="middle">
              {t('banner.diagram.records')}
            </text>
            <text x="370" y="90" textAnchor="middle">
              {t('banner.diagram.server')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
