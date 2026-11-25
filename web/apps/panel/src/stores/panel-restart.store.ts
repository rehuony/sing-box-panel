import { create } from 'zustand';

interface PanelRestartState {
  /** A restart was asked for and the restarted panel has not answered yet. */
  restarting: boolean;
  setRestarting: (restarting: boolean) => void;
}

/** The panel's restart, shared by the status line's action and the command line's `reboot`. */
export const usePanelRestart = create<PanelRestartState>()(set => ({
  restarting: false,
  setRestarting: restarting => set({ restarting }),
}));
