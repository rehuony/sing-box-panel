export interface PanelLogoProps {
  compact?: boolean;
}

export function PanelLogo({ compact = false }: PanelLogoProps) {
  return (
    <span className="panel-logo">
      <svg
        aria-hidden="true"
        className="panel-logo__mark"
        viewBox="0 0 32 32"
      >
        <path d="M5 9.5 16 3l11 6.5v13L16 29 5 22.5Z" />
        <path d="m10 12 6-3.5 6 3.5v7l-6 3.5-6-3.5Z" />
        <path d="M5 9.5 10 12m17-2.5L22 12M16 29v-6.5" />
      </svg>
      {compact ? null : (
        <span className="panel-logo__wordmark">
          <strong>sing-box</strong>
          <span>panel</span>
        </span>
      )}
    </span>
  );
}
