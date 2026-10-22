import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export function GitConnectionsDiagram({
  size = 'hero',
}: {
  size?: 'hero' | 'compact'
}) {
  const { t } = useTranslation('settings')
  const hero = size === 'hero'
  return (
    <svg
      viewBox={hero ? '0 0 520 300' : '40 70 440 160'}
      className="h-full w-full max-w-full"
      fill="none"
      role="presentation"
    >
      <IsoFloor />
      <g strokeWidth="1.5" strokeDasharray="4 4">
        <path d="M110 122L236 150" className="stroke-(--iso-sleep-r)" />
        <path d="M110 202L236 172" className="stroke-(--iso-sleep-r)" />
        <path d="M324 150L418 126" className="stroke-(--iso-accent-l)" />
        <path d="M324 172L418 198" className="stroke-(--iso-accent-l)" />
      </g>
      <Block x={84} y={116} a={26} h={12} p={GREY} />
      <Block x={84} y={196} a={26} h={12} p={GREY} />
      <Block x={280} y={160} a={40} h={26} p={BLUE} />
      <circle cx="250" cy="182" r="1.8" className="fill-card" />
      <circle cx="250" cy="194" r="1.8" className="fill-card" />
      <Block x={436} y={120} a={14} h={10} p={YELLOW} />
      <Block x={436} y={196} a={14} h={10} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M84 96V74" />
            <path d="M280 134V74" />
            <path d="M436 102V74" />
          </g>
          <g className={LABEL}>
            <text x="84" y="64" textAnchor="middle">
              {t('gitConnectionsHub.banner.diagram.repos')}
            </text>
            <text x="280" y="64" textAnchor="middle">
              {t('gitConnectionsHub.banner.diagram.server')}
            </text>
            <text x="436" y="64" textAnchor="middle">
              {t('gitConnectionsHub.banner.diagram.branches')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
