import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export function FunctionsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('functions')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '130 85 290 115'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4">
        <path d="M88 150L154 117L220 150" className="stroke-(--iso-accent-l)" />
        <path d="M300 150L341 129.5L382 150" className="stroke-(--iso-l)" />
      </g>
      <Block x={60} y={150} a={28} h={14} p={GREY} />
      <rect
        x="51"
        y="145"
        width="18"
        height="11"
        rx="1.5"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      <path
        d="M51 148H69"
        className="stroke-(--iso-sleep-r)"
        strokeWidth="1.2"
      />
      <Block x={154} y={110} a={11} h={11} p={YELLOW} />
      <Block x={260} y={150} a={40} h={28} p={BLUE} />
      <path
        d="M264 136L253 152H260L257 164L269 147H262Z"
        className="fill-card stroke-(--iso-r)"
        strokeWidth="1"
        strokeLinejoin="round"
      />
      <Block x={410} y={150} a={28} h={14} p={BLUE} />
      <Block x={330} y={225} a={34} h={22} p={GREY} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M60 128V98" />
            <path d="M154 96V76" />
            <path d="M260 124V60" />
            <path d="M410 134V98" />
            <path d="M330 262V276" />
            <path d="M268 126L336 44" strokeDasharray="2 3" />
          </g>
          <rect
            x="336"
            y="24"
            width="170"
            height="26"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL} textAnchor="middle">
            <text x="60" y="90">
              {t('banner.diagram.browser')}
            </text>
            <text x="154" y="68">
              {t('banner.diagram.request')}
            </text>
            <text x="260" y="52">
              {t('banner.diagram.function')}
            </text>
            <text x="410" y="90">
              {t('banner.diagram.response')}
            </text>
            <text x="421" y="42" className="fill-foreground">
              {t('banner.diagram.coldStart')}
            </text>
            <text x="330" y="292">
              {t('banner.diagram.sleeps')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
