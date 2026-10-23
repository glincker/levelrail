import { useTranslation } from 'react-i18next'
import { Block, IsoFloor } from './iso'
import { BLUE, GREY, LABEL, YELLOW } from './isoTokens'

export default function RegistryDiagram({
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
      <g strokeWidth="1.5" strokeDasharray="4 4" className="stroke-(--iso-l)">
        <path d="M142 168L208 150" />
        <path d="M312 148L370 168" />
      </g>
      <Block x={120} y={175} a={24} h={10} p={GREY} />
      <Block x={260} y={140} a={54} h={16} p={BLUE} />
      <Block x={260} y={124} a={54} h={16} p={BLUE} />
      <Block x={390} y={175} a={24} h={10} p={GREY} />
      <Block x={176} y={159} a={8} h={8} p={YELLOW} />
      <Block x={340} y={158} a={8} h={8} p={YELLOW} />
      {hero ? (
        <>
          <g className="stroke-border" strokeWidth="1">
            <path d="M120 148V160" />
            <path d="M390 148V160" />
            <path d="M260 186V230" strokeDasharray="2 3" />
          </g>
          <rect
            x="190"
            y="230"
            width="140"
            height="28"
            rx="11"
            className="fill-card stroke-border"
          />
          <g className={LABEL}>
            <text x="120" y="140" textAnchor="middle">
              {t('builtInRegistryPage.banner.diagram.push')}
            </text>
            <text x="390" y="140" textAnchor="middle">
              {t('builtInRegistryPage.banner.diagram.pull')}
            </text>
            <text
              x="260"
              y="249"
              textAnchor="middle"
              className="fill-foreground"
            >
              {t('builtInRegistryPage.banner.diagram.registry')}
            </text>
          </g>
        </>
      ) : null}
    </svg>
  )
}
