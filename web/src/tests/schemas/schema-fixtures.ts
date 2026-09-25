import manifest from '@/schemas/generated/manifest.json';

// Exact-version loading is tested separately; editor behavior depends on schema content.
export function representativeSchemaVersions(source?: string): string[] {
  const seen = new Set<string>();
  const versions: string[] = [];
  for (const entry of manifest.entries) {
    if (source !== undefined && entry.source !== source) continue;
    const key = `${entry.source}:${entry.schema_sha256}`;
    if (seen.has(key)) continue;
    seen.add(key);
    versions.push(entry.exact_version);
  }
  if (versions.length === 0) throw new Error(`Missing committed schemas for ${source ?? 'all sources'}.`);
  return versions;
}
