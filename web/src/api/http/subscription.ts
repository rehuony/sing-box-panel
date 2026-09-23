import type { HttpApiContext } from './shared';
import type { ApiClient, CreatedSubscriptionToken, SubscriptionChannel, SubscriptionChannelPage, SubscriptionNodeCatalog, SubscriptionNodeDetail, SubscriptionNodeSummary, SubscriptionPreview, SubscriptionSource, SubscriptionSourcePage, SubscriptionSourceRefreshResult, SubscriptionSourceVersion, SubscriptionSourceVersionPage, SubscriptionSourceVersionSave, SubscriptionToken, SubscriptionTokenPage, SubscriptionTokenRotation, SubscriptionUser, SubscriptionUserGrants, SubscriptionUserPage } from '../api-client';

function utf8Base64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}
export function createSubscriptionHttpApi(context: HttpApiContext) {
  const {
    baseUrl, buildQuery, fetcher, quoteETag, request, writeHeaders, writeJSONHeaders,
  } = context;
  return {
    listSubscriptionChannels(filter = {}, signal) {
      const query = buildQuery({
        limit: filter.limit ?? 50,
        before_time: filter.beforeTime,
        before_id: filter.beforeID,
      });
      return request<SubscriptionChannelPage>(
        fetcher,
        `${baseUrl}/subscription/channels${query}`,
        { method: 'GET', signal },
      );
    },
    getSubscriptionChannel(channelID, signal) {
      return request<SubscriptionChannel>(
        fetcher,
        `${baseUrl}/subscription/channels/${encodeURIComponent(channelID)}`,
        { method: 'GET', signal },
      );
    },
    createSubscriptionChannel(input, signal) {
      return request<SubscriptionChannel>(
        fetcher,
        `${baseUrl}/subscription/channels`,
        {
          method: 'POST',
          body: JSON.stringify(input),
          headers: writeJSONHeaders(),
          signal,
        },
      );
    },
    updateSubscriptionChannel(channelID, input, updatedAt, signal) {
      return request<SubscriptionChannel>(
        fetcher,
        `${baseUrl}/subscription/channels/${encodeURIComponent(channelID)}`,
        {
          method: 'PUT',
          body: JSON.stringify(input),
          headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    deleteSubscriptionChannel(channelID, updatedAt, signal) {
      return request<void>(
        fetcher,
        `${baseUrl}/subscription/channels/${encodeURIComponent(channelID)}`,
        {
          method: 'DELETE',
          headers: writeHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    async previewSubscriptionChannel(channelID, userID, signal, draft) {
      const preview = await request<SubscriptionPreview>(
        fetcher,
        `${baseUrl}/subscription/channels/${encodeURIComponent(channelID)}/preview`,
        {
          method: 'POST',
          body: JSON.stringify({ user_id: userID, draft }),
          headers: writeJSONHeaders(),
          signal,
        },
      );
      // OpenAPI format: byte is Base64 on the wire; UI/demo contracts expose UTF-8 text.
      const bytes = Uint8Array.from(atob(preview.result.content), value => value.charCodeAt(0));
      return { ...preview, result: { ...preview.result, content: new TextDecoder('utf-8', { fatal: true }).decode(bytes) } };
    },
    listSubscriptionUsers(filter = {}, signal) {
      const query = buildQuery({
        limit: filter.limit ?? 50,
        before_time: filter.beforeTime,
        before_id: filter.beforeID,
      });
      return request<SubscriptionUserPage>(fetcher, `${baseUrl}/subscription/users${query}`, {
        method: 'GET',
        signal,
      });
    },
    getSubscriptionUser(userID, signal) {
      return request<SubscriptionUser>(fetcher, `${baseUrl}/subscription/users/${encodeURIComponent(userID)}`, {
        method: 'GET',
        signal,
      });
    },
    createSubscriptionUser(input, signal) {
      return request<SubscriptionUser>(fetcher, `${baseUrl}/subscription/users`, {
        method: 'POST',
        body: JSON.stringify(input),
        headers: writeJSONHeaders(),
        signal,
      });
    },
    updateSubscriptionUser(userID, input, updatedAt, signal) {
      return request<SubscriptionUser>(fetcher, `${baseUrl}/subscription/users/${encodeURIComponent(userID)}`, {
        method: 'PUT',
        body: JSON.stringify(input),
        headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }),
        signal,
      });
    },
    deleteSubscriptionUser(userID, updatedAt, signal) {
      return request<void>(fetcher, `${baseUrl}/subscription/users/${encodeURIComponent(userID)}`, {
        method: 'DELETE',
        headers: writeHeaders({ 'If-Match': quoteETag(updatedAt) }),
        signal,
      });
    },
    getSubscriptionNode(id, signal) {
      return request<SubscriptionNodeDetail>(fetcher, `${baseUrl}/subscription/nodes/${encodeURIComponent(id)}`, { signal });
    },
    createSubscriptionNode(outboundJSON, signal) {
      return request<SubscriptionNodeDetail>(fetcher, `${baseUrl}/subscription/nodes`, {
        method: 'POST', body: `{ "outbound": ${outboundJSON} }`, headers: writeJSONHeaders(), signal,
      });
    },
    updateSubscriptionNode(id, outboundJSON, revision, signal) {
      return request<SubscriptionNodeDetail>(fetcher, `${baseUrl}/subscription/nodes/${encodeURIComponent(id)}`, {
        method: 'PUT', body: `{ "outbound": ${outboundJSON}, "revision": ${JSON.stringify(revision)} }`,
        headers: writeJSONHeaders(), signal,
      });
    },
    deleteSubscriptionNode(id, revision, signal) {
      return request<void>(fetcher, `${baseUrl}/subscription/nodes/${encodeURIComponent(id)}`, {
        method: 'DELETE', body: JSON.stringify({ revision }), headers: writeJSONHeaders(), signal,
      });
    },
    setSubscriptionNodeVisibility(id, hidden, revision, signal) {
      return request<SubscriptionNodeSummary>(fetcher, `${baseUrl}/subscription/nodes/${encodeURIComponent(id)}/visibility`, {
        method: 'PUT', body: JSON.stringify({ hidden, revision }), headers: writeJSONHeaders(), signal,
      });
    },
    parseSubscriptionNode(text, signal) {
      return request<{ outbound_json: string }>(fetcher, `${baseUrl}/subscription/nodes/parse`, {
        method: 'POST', body: JSON.stringify({ text }), headers: writeJSONHeaders(), signal,
      });
    },
    getSubscriptionNodeCatalog(signal) {
      return request<SubscriptionNodeCatalog>(fetcher, `${baseUrl}/subscription/nodes`, {
        method: 'GET',
        signal,
      });
    },
    getSubscriptionUserGrants(userID, signal) {
      return request<SubscriptionUserGrants>(fetcher, `${baseUrl}/subscription/users/${encodeURIComponent(userID)}/grants`, {
        method: 'GET',
        signal,
      });
    },
    replaceSubscriptionUserGrants(userID, grants, updatedAt, signal) {
      return request<SubscriptionUserGrants>(fetcher, `${baseUrl}/subscription/users/${encodeURIComponent(userID)}/grants`, {
        method: 'PUT',
        body: JSON.stringify({ grants }),
        headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }),
        signal,
      });
    },
    listSubscriptionSources(filter = {}, signal) {
      const query = buildQuery({
        limit: filter.limit ?? 50,
        before_time: filter.beforeTime,
        before_id: filter.beforeID,
      });
      return request<SubscriptionSourcePage>(
        fetcher,
        `${baseUrl}/subscription/sources${query}`,
        { method: 'GET', signal },
      );
    },
    getSubscriptionSource(sourceID, signal) {
      return request<SubscriptionSource>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}`,
        { method: 'GET', signal },
      );
    },
    createSubscriptionSource(input, signal) {
      return request<SubscriptionSource>(fetcher, `${baseUrl}/subscription/sources`, {
        method: 'POST',
        body: JSON.stringify(input),
        headers: writeJSONHeaders(),
        signal,
      });
    },
    updateSubscriptionSource(sourceID, input, updatedAt, signal) {
      return request<SubscriptionSource>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}`,
        {
          method: 'PUT',
          body: JSON.stringify(input),
          headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    refreshSubscriptionSource(sourceID, signal) {
      return request<SubscriptionSourceRefreshResult>(fetcher, `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}/refresh`, {
        method: 'POST',
        headers: writeHeaders(),
        signal,
      });
    },
    listSubscriptionSourceVersions(sourceID, filter = {}, signal) {
      const query = buildQuery({
        limit: filter.limit ?? 50,
        before_time: filter.beforeTime,
        before_id: filter.beforeID,
      });
      return request<SubscriptionSourceVersionPage>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}/versions${query}`,
        { method: 'GET', signal },
      );
    },
    getSubscriptionSourceVersion(sourceID, versionID, signal) {
      return request<SubscriptionSourceVersion>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}/versions/${encodeURIComponent(versionID)}`,
        { method: 'GET', signal },
      );
    },
    createSubscriptionSourceVersion(sourceID, format, rawBody, updatedAt, signal) {
      return request<SubscriptionSourceVersionSave>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}/versions`,
        {
          method: 'POST',
          body: JSON.stringify({ format, raw_body: utf8Base64(rawBody) }),
          headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    restoreSubscriptionSourceVersion(sourceID, versionID, updatedAt, signal) {
      return request<SubscriptionSource>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}/versions/${encodeURIComponent(versionID)}/restore`,
        {
          method: 'POST',
          headers: writeHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    deleteSubscriptionSource(sourceID, updatedAt, signal) {
      return request<void>(
        fetcher,
        `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}`,
        {
          method: 'DELETE',
          headers: writeHeaders({ 'If-Match': quoteETag(updatedAt) }),
          signal,
        },
      );
    },
    listSubscriptionTokens(filter = {}, signal) {
      const query = buildQuery({
        offset: filter.offset,
        limit: filter.limit ?? 50,
        before_time: filter.beforeTime,
        before_id: filter.beforeID,
      });
      return request<SubscriptionTokenPage>(fetcher, `${baseUrl}/subscription/tokens${query}`, {
        method: 'GET',
        signal,
      });
    },
    getSubscriptionTokenSecret(tokenID, signal) {
      return request<{ token: string }>(fetcher, `${baseUrl}/subscription/tokens/${encodeURIComponent(tokenID)}/secret`, { method: 'GET', signal });
    },
    getSubscriptionToken(tokenID, signal) {
      return request<SubscriptionToken>(
        fetcher,
        `${baseUrl}/subscription/tokens/${encodeURIComponent(tokenID)}`,
        { method: 'GET', signal },
      );
    },
    createSubscriptionToken(input, signal) {
      return request<CreatedSubscriptionToken>(
        fetcher,
        `${baseUrl}/subscription/tokens`,
        {
          method: 'POST',
          body: JSON.stringify({
            user_id: input.userID,
            label: input.label,
            expires_at: input.expiresAt,
            download_limit: input.downloadLimit,
          }),
          headers: writeJSONHeaders(),
          signal,
        },
      );
    },
    rotateSubscriptionToken(tokenID, expiresAt, signal) {
      return request<SubscriptionTokenRotation>(
        fetcher,
        `${baseUrl}/subscription/tokens/${encodeURIComponent(tokenID)}/rotate`,
        {
          method: 'POST',
          body: JSON.stringify({ expires_at: expiresAt }),
          headers: writeJSONHeaders(),
          signal,
        },
      );
    },
    revokeSubscriptionToken(tokenID, signal) {
      return request<SubscriptionToken>(
        fetcher,
        `${baseUrl}/subscription/tokens/${encodeURIComponent(tokenID)}/revoke`,
        { method: 'POST', headers: writeHeaders(), signal },
      );
    },
    setSubscriptionTokenEnabled(tokenID, enabled, signal) {
      return request<SubscriptionToken>(
        fetcher,
        `${baseUrl}/subscription/tokens/${encodeURIComponent(tokenID)}/${enabled ? 'enable' : 'disable'}`,
        { method: 'POST', headers: writeHeaders(), signal },
      );
    },
    deleteSubscriptionToken(tokenID, signal) {
      return request<void>(fetcher, `${baseUrl}/subscription/tokens/${encodeURIComponent(tokenID)}`, {
        method: 'DELETE',
        headers: writeHeaders(),
        signal,
      });
    },

  } satisfies Partial<ApiClient>;
}
