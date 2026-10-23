import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

function StorageSlab({ x, y }: { x: number; y: number }) {
  return (
    <g>
      <Block x={x} y={y} a={40} h={12} p={BLUE} />
      <Block x={x} y={y - 16} a={40} h={12} p={BLUE} />
      <circle cx={x - 30} cy={y - 4} r="1.8" className="fill-card" />
      <circle cx={x - 30} cy={y + 12} r="1.8" className="fill-card" />
    </g>
  )
}

export function NetworkSharesDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('settings')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '70 55 350 150'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4">
        <path d="M138 176L300 95" className="stroke-(--iso-l)" />
        <path d="M236 123L330 170" className="stroke-(--iso-l)" />
      </g>
      <StorageSlab x={340} y={91} />
      <Block x={236} y={104} a={10} h={10} p={YELLOW} />
      <StorageSlab x={370} y={170} />
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
            <path d="M382 91H390" />
            <path d="M412 170H420" />
            <path d="M283 146V230" strokeDasharray="2 3" />
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
              {t('networkSharesPage.banner.diagram.app')}
            </text>
            <text x="394" y="96">
              {t('networkSharesPage.banner.diagram.nfs')}
            </text>
            <text x="424" y="175">
              {t('networkSharesPage.banner.diagram.smb')}
            </text>
            <text
              x="283"
              y="249"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('networkSharesPage.banner.diagram.volume')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
