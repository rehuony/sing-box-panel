export interface PanelLogoProps {
  compact?: boolean;
}

const panelLogoSource = `${import.meta.env.BASE_URL}favicon.svg`;

export function PanelLogo({ compact = false }: PanelLogoProps) {
  return (
    <span aria-label={compact ? 'Sing-Box Panel' : undefined} className='panel-logo'>
      <img
        alt=''
        aria-hidden='true'
        className='panel-logo__mark'
        height='40'
        src={panelLogoSource}
        width='40'
      />
      {compact
        ? null
        : (
            <span className='panel-logo__wordmark'>
              <strong>Sing-Box Panel</strong>
            </span>
          )}
    </span>
  );
}
