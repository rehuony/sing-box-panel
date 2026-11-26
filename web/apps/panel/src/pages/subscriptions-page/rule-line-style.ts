export const dataFace = 'font-[family-name:var(--font-data)]';
/** A gutter cell: a line number or a prompt, right-aligned in the data face. */
export const gutterCell = `text-end ${dataFace} text-xs leading-5 tabular-nums`;

/** The gutter's width for a list this long: its numbers in the data face, at least two digits wide. */
export function gutterWidth(lines: number): string {
  // The data face advances 0.6em, 0.45rem at 12px.
  return `${Math.max(2, String(lines).length) * 0.45}rem`;
}
