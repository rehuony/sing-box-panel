export interface Session {
  displayName: string;
}

export interface DashboardContext {
  view: {
    exactVersion: string;
  };
  adapter: {
    supported: boolean;
    label: string;
    warning: string | null;
  };
  applied: {
    bundle: string;
    revision: number;
    appliedAt: string;
  } | null;
  canonical: {
    revision: number;
    savedAt: string;
    hasUnappliedChanges: boolean;
  };
  running: {
    exactVersion: string;
    artifactName: string;
    digest: string;
  } | null;
}
