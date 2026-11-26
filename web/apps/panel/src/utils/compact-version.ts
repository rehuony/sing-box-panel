/**
 * A version short enough for a title bar or the status line. One with a long
 * pre-release or build part, such as a development build's pseudo-version
 * (`v0.5.5-0.20261126051431-9230819beb95+dirty`), keeps its release and the
 * commit it was built from, joined by an ellipsis: `v0.5.5…9230819`. Where it
 * is shortened, the whole version belongs in its tooltip.
 */
export function compactVersion(version: string): string {
  const match = /^(v?\d+\.\d+\.\d+)([-+].+)$/.exec(version);
  if (!match || match[2].length <= 13) return version;
  const prerelease = match[2].split('+')[0];
  const commit = /([0-9a-f]{12})$/i.exec(prerelease)?.[1].slice(0, 7);
  return `${match[1]}…${commit ?? (prerelease || match[2]).slice(-7)}`;
}
