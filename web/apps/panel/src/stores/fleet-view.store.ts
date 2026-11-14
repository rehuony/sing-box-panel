import { create } from 'zustand';

import type { StateFilter } from '@/utils/panel-commands';

export type FleetStateFilter = 'all' | StateFilter;

interface FleetViewState {
  search: string;
  tag: string | null;
  /** Where a Shift+click range starts. */
  anchor: string | null;
  /** The home screen's filters, which the command line sets too. */
  state: FleetStateFilter;
  clearFilters: () => void;
  clearSelection: () => void;
  /** The nodes the home screen shows, in its order, which select all and ranges follow. */
  visible: readonly string[];
  /** Selected node IDs, in the order they were selected. */
  selected: ReadonlySet<string>;
  select: (ids: readonly string[]) => void;
  setVisible: (ids: readonly string[]) => void;
  /**
   * Toggles one node; with a range, selects every node from the anchor to it
   * in the order given, which is the order the screen shows.
   */
  toggle: (id: string, range?: readonly string[]) => void;
  setFilter: (filter: Partial<Pick<FleetViewState, 'search' | 'state' | 'tag'>>) => void;
}

export const useFleetView = create<FleetViewState>()(set => ({
  anchor: null,
  clearFilters: () => set({ search: '', state: 'all', tag: null }),
  clearSelection: () => set({ anchor: null, selected: new Set() }),
  search: '',
  select: ids => set(state => ({ selected: new Set([...state.selected, ...ids]) })),
  selected: new Set(),
  setFilter: filter => set(filter),
  setVisible: visible => set({ visible }),
  state: 'all',
  tag: null,
  visible: [],
  toggle: (id, range) => set((state) => {
    const start = state.anchor === null || range === undefined ? -1 : range.indexOf(state.anchor);
    const end = range?.indexOf(id) ?? -1;
    if (start >= 0 && end >= 0) {
      const ids = range!.slice(Math.min(start, end), Math.max(start, end) + 1);
      return { anchor: id, selected: new Set([...state.selected, ...ids]) };
    }
    const selected = new Set(state.selected);
    if (selected.has(id)) selected.delete(id);
    else selected.add(id);
    return { anchor: id, selected };
  }),
}));
