import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function DomainsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('domains')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '40 60 420 170'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4">
        <path d="M104 144L204 94" className="stroke-(--iso-sleep-r)" />
        <path d="M256 100L376 160" className="stroke-(--iso-l)" />
      </g>
      <Block x={80} y={150} a={28} h={14} p={GREY} />
      <rect
        x="71"
        y="145"
        width="18"
        height="11"
        rx="1.5"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      <path
        d="M71 148H89"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      <Block x={230} y={100} a={26} h={12} p={YELLOW} />
      <g
        className="stroke-(--iso-accent-r)"
        strokeWidth="1.3"
        strokeLinecap="round"
      >
        <rect
          x="225"
          y="98"
          width="10"
          height="7"
          rx="1.2"
          className="fill-card"
        />
        <path d="M227 98V95.5a3 3 0 0 1 6 0V98" />
      </g>
      <Block x={420} y={170} a={44} h={24} p={BLUE} />
      <g className="stroke-card" strokeWidth="1.2" strokeLinecap="round">
        <path d="M388 186L418 201" />
        <path d="M388 194L418 209" />
      </g>
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M100 96V130" strokeDasharray="2 3" />
            <path d="M230 78V60" />
            <path d="M420 218V236" />
            <path d="M80 178V196" />
          </g>
          <rect
            x="25"
            y="70"
            width="150"
            height="26"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL} textAnchor="middle">
            <text x="100" y="88" className="fill-foreground">
              {t('banner.diagram.domain')}
            </text>
            <text x="230" y="52">
              {t('banner.diagram.certificate')}
            </text>
            <text x="420" y="256">
              {t('banner.diagram.server')}
            </text>
            <text x="80" y="214">
              {t('banner.diagram.browser')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
