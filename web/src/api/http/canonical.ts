import type { HttpApiContext } from './shared';
import type { ApiClient, CanonicalChange, CanonicalRevisionDiff, CanonicalRevisionPage, CanonicalSave, CanonicalSnapshot } from '../api-client';

export function createCanonicalHttpApi(context: HttpApiContext) {
  const {
    baseUrl, buildQuery, fetcher, quoteETag, request, writeHeaders, writeJSONHeaders,
  } = context;
  return {
    getCanonical(signal) {
      return request<CanonicalSnapshot>(fetcher, `${baseUrl}/config/canonical`, {
        method: 'GET', signal,
      });
    },
    replaceCanonical(documentJSON: string, baseRevision, signal) {
      return request<CanonicalSave>(fetcher, `${baseUrl}/config/canonical`, {
        method: 'PUT',
        body: documentJSON,
        headers: writeJSONHeaders({ 'If-Match': quoteETag(baseRevision) }),
        signal,
      });
    },
    patchCanonical(changes: CanonicalChange[], baseRevision, signal) {
      return request<CanonicalSave>(fetcher, `${baseUrl}/config/canonical`, {
        method: 'PATCH',
        body: JSON.stringify({ changes }),
        headers: writeJSONHeaders({ 'If-Match': quoteETag(baseRevision) }),
        signal,
      });
    },
    listRevisions(signal) {
      return request<CanonicalRevisionPage>(fetcher, `${baseUrl}/config/revisions?limit=8`, {
        method: 'GET', signal,
      });
    },
    getRevision(reference, signal) {
      return request<CanonicalSnapshot>(fetcher, `${baseUrl}/config/revisions/${encodeURIComponent(reference)}`, {
        method: 'GET', signal,
      });
    },
    diffRevisions(from, to, signal) {
      const query = buildQuery({ from, to });
      return request<CanonicalRevisionDiff>(fetcher, `${baseUrl}/config/revisions/diff${query}`, {
        method: 'GET', signal,
      });
    },
    restoreRevision(reference, baseRevision, signal) {
      return request<CanonicalSave>(fetcher, `${baseUrl}/config/revisions/${encodeURIComponent(reference)}/restore`, {
        method: 'POST', headers: writeHeaders({ 'If-Match': quoteETag(baseRevision) }), signal,
      });
    },
  } satisfies Partial<ApiClient>;
}
