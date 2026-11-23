import type { ApiClient, CoreArtifact, SystemStatus } from '@/api/api-client';

type Platform = NonNullable<SystemStatus['platform']>;

export async function listInstalledCoreArtifacts(
  client: Pick<ApiClient, 'listCoreArtifacts'>,
  platform: Platform,
  signal: AbortSignal,
): Promise<CoreArtifact[]> {
  const items: CoreArtifact[] = [];
  let next: { created_at: string; id: string } | undefined;
  do {
    const page = await client.listCoreArtifacts({
      architecture: platform.arch,
      limit: 200,
      beforeID: next?.id,
      beforeTime: next?.created_at,
    }, signal);
    items.push(...page.items);
    next = page.next;
  } while (next !== undefined && !signal.aborted);

  return items.filter(artifact => artifact.os === platform.os && artifact.arch === platform.arch);
}

function compareExactVersions(left: string, right: string): number {
  const leftParts = left.split('.').map(Number);
  const rightParts = right.split('.').map(Number);
  for (let index = 0; index < 3; index += 1) {
    const difference = (rightParts[index] ?? 0) - (leftParts[index] ?? 0);
    if (difference !== 0) return difference;
  }
  return 0;
}

export function installedCoreVersions(artifacts: CoreArtifact[]): string[] {
  return [...new Set(artifacts.map(artifact => artifact.exact_version))].sort(compareExactVersions);
}
