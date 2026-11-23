import { useLocation, useNavigate } from 'react-router-dom';

export function useHashTab<T extends string>(
  prefix: string,
  values: readonly T[],
  fallback: T,
): [T, (value: string) => void] {
  const location = useLocation();
  const navigate = useNavigate();
  const candidate = location.hash.startsWith(`#${prefix}`)
    ? location.hash.slice(prefix.length + 1).split('/')[0]
    : '';
  const value = values.includes(candidate as T) ? candidate as T : fallback;
  return [value, (next) => {
    if (!values.includes(next as T)) return;
    const hash = `#${prefix}${next}`;
    if (location.hash === hash) return;
    void navigate({ pathname: location.pathname, search: location.search, hash }, { state: location.state });
  }];
}
