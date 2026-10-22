import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

function DatabaseStack({ x, y }: { x: number; y: number }) {
  return (
    <g>
      {[0, 1, 2].map((i) => (
        <g key={i}>
          <Block x={x} y={y - i * 17} a={40} h={11} p={BLUE} />
          <circle
            cx={x - 29}
            cy={y - i * 17 + 3}
            r="1.8"
            className="fill-card"
          />
          <circle
            cx={x - 22}
            cy={y - i * 17 + 6.5}
            r="1.8"
            className="fill-card"
          />
        </g>
      ))}
    </g>
  )
}

export function DatabasesDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('databases')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '60 46 420 168'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4">
        <path d="M132 170L258 107" className="stroke-(--iso-l)" />
        <path d="M346 125L384 144" className="stroke-(--iso-l)" />
      </g>
      <path
        d="M378 135L385 145L374 146"
        className="stroke-(--iso-l)"
        strokeWidth="1.5"
      />
      <DatabaseStack x={300} y={112} />
      <Block x={198} y={130} a={9} h={9} p={YELLOW} />
      <Block x={420} y={168} a={30} h={14} p={BLUE} />
      <Block x={105} y={174} a={30} h={16} p={GREY} />
      <rect
        x="94"
        y="168"
        width="22"
        height="12"
        rx="1.5"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      <path
        d="M94 171H116"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M105 156V136" />
            <path d="M300 62V50" />
            <path d="M283 150V230" strokeDasharray="2 3" />
          </g>
          <rect
            x="150"
            y="230"
            width="266"
            height="28"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL}>
            <text x="105" y="128" textAnchor="middle">
              {t('banner.diagram.app')}
            </text>
            <text x="300" y="44" textAnchor="middle">
              {t('banner.diagram.database')}
            </text>
            <text x="426" y="204" textAnchor="middle">
              {t('banner.diagram.backups')}
            </text>
            <text
              x="283"
              y="249"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('banner.diagram.restore')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
