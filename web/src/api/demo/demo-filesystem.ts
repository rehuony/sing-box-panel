import type { ApiClient, FilesystemEntry } from '../api-client';

import { ApiRequestError } from '../api-client';

const base = '/var/lib/sing-box-panel/runtime';

function normalize(path: string): string {
  const parts: string[] = [];
  for (const part of (path.startsWith('/') ? path : `${base}/${path}`).split('/')) {
    if (part === '..') parts.pop();
    else if (part && part !== '.') parts.push(part);
  }
  return `/${parts.join('/')}`;
}

function parent(path: string): string {
  return path.slice(0, path.lastIndexOf('/')) || '/';
}

export function createDemoFilesystemApi() {
  const entries = new Map<string, FilesystemEntry>();
  for (const [kind, paths] of Object.entries({
    directory: ['/', '/etc', '/etc/sing-box', '/var', '/var/lib', '/var/lib/sing-box-panel', base, `${base}/rules`, `${base}/logs`],
    file: ['/etc/sing-box/certificate.pem', '/etc/sing-box/private.key', `${base}/rules/local.srs`, `${base}/.hidden`, `${base}/cache.db`],
    socket: [`${base}/protect.sock`],
  })) {
    for (const path of paths) entries.set(path, { name: path.split('/').at(-1) || '/', path, kind: kind as FilesystemEntry['kind'], available: true, symlink: false });
  }
  function fail(code: string, status: number): never {
    throw new ApiRequestError('The demo path cannot be selected.', { code, status });
  }
  return {
    async listFilesystemEntries(query = {}, signal) {
      signal?.throwIfAborted();
      const requested = normalize(query.path || base);
      let directory = requested;
      let fallback = false;
      while (entries.get(directory)?.kind !== 'directory') {
        if (!entries.has(directory)) fallback = true;
        directory = parent(directory);
      }
      const items = [...entries.values()].filter(entry => entry.path !== '/' && parent(entry.path) === directory
        && (query.show_hidden || !entry.name.startsWith('.'))
        && entry.name.toLowerCase().includes((query.search || '').toLowerCase())).sort((a, b) => Number(b.kind === 'directory') - Number(a.kind === 'directory') || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
      const limit = query.limit ?? 50;
      const offset = Math.min(query.offset ?? 0, Math.max(0, Math.ceil(items.length / limit) - 1) * limit);
      return {
        path: directory, parent: parent(directory), requested_path: requested, fallback,
        items: items.slice(offset, offset + limit), total: items.length, offset, limit,
      };
    },
    async resolveFilesystemPath(input, signal) {
      signal?.throwIfAborted();
      const path = normalize(input.path);
      const entry = entries.get(path);
      const expected = input.mode === 'output-file' ? 'file' : input.mode;
      if (entry && entry.kind !== expected) fail('filesystem_type_mismatch', 422);
      if (!entry && (input.mode !== 'output-file' || entries.get(parent(path))?.kind !== 'directory')) fail('filesystem_not_found', 404);
      return { path, parent: parent(path), kind: expected, exists: !!entry, symlink: false };
    },
  } satisfies Pick<ApiClient, 'listFilesystemEntries' | 'resolveFilesystemPath'>;
}
