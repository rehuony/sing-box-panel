import type { HttpApiContext } from './shared';
import type {
  ApiClient,
  CreatedSubscriptionToken,
  SubscriptionChannel,
  SubscriptionChannelPage,
  SubscriptionNodeDetail,
  SubscriptionNodeList,
  SubscriptionPreview,
  SubscriptionRuleSet,
  SubscriptionRuleSetList,
  SubscriptionSource,
  SubscriptionSourcePage,
  SubscriptionSourcePreview,
  SubscriptionToken,
  SubscriptionTokenPage,
  SubscriptionTokenRotation,
} from '../api-client';

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
      // OpenAPI format: byte is Base64 on the wire; the UI contract exposes UTF-8 text.
      const bytes = Uint8Array.from(atob(preview.result.content), value => value.charCodeAt(0));
      return { ...preview, result: { ...preview.result, content: new TextDecoder('utf-8', { fatal: true }).decode(bytes) } };
    },
    listSubscriptionNodes(signal) {
      return request<SubscriptionNodeList>(fetcher, `${baseUrl}/subscription/nodes`, { method: 'GET', signal });
    },
    setSubscriptionNodeOrder(ids, signal) {
      return request<SubscriptionNodeList>(fetcher, `${baseUrl}/subscription/node-order`, {
        method: 'PUT', body: JSON.stringify({ ids }), headers: writeJSONHeaders(), signal,
      });
    },
    getSubscriptionNode(id, signal) {
      return request<SubscriptionNodeDetail>(fetcher, `${baseUrl}/subscription/nodes/${encodeURIComponent(id)}`, { method: 'GET', signal });
    },
    updateSubscriptionNode(id, input, updatedAt, signal) {
      return request<SubscriptionNodeDetail>(fetcher, `${baseUrl}/subscription/nodes/${encodeURIComponent(id)}`, {
        method: 'PUT', body: JSON.stringify(input), headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }), signal,
      });
    },
    listSubscriptionRuleSets(client, signal) {
      return request<SubscriptionRuleSetList>(fetcher, `${baseUrl}/subscription/rule-sets${buildQuery({ client })}`, { method: 'GET', signal });
    },
    setSubscriptionRuleSetOrder(client, ids, signal) {
      return request<void>(fetcher, `${baseUrl}/subscription/rule-set-order`, {
        method: 'PUT', body: JSON.stringify({ client, ids }), headers: writeJSONHeaders(), signal,
      });
    },
    createSubscriptionRuleSet(input, signal) {
      return request<SubscriptionRuleSet>(fetcher, `${baseUrl}/subscription/rule-sets`, {
        method: 'POST', body: JSON.stringify(input), headers: writeJSONHeaders(), signal,
      });
    },
    updateSubscriptionRuleSet(ruleSetID, input, updatedAt, signal) {
      return request<SubscriptionRuleSet>(fetcher, `${baseUrl}/subscription/rule-sets/${encodeURIComponent(ruleSetID)}`, {
        method: 'PUT', body: JSON.stringify(input), headers: writeJSONHeaders({ 'If-Match': quoteETag(updatedAt) }), signal,
      });
    },
    deleteSubscriptionRuleSet(ruleSetID, updatedAt, signal) {
      return request<void>(fetcher, `${baseUrl}/subscription/rule-sets/${encodeURIComponent(ruleSetID)}`, {
        method: 'DELETE', headers: writeHeaders({ 'If-Match': quoteETag(updatedAt) }), signal,
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
    deleteSubscriptionSource(sourceID, updatedAt, signal) {
      return request<void>(fetcher, `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}`, {
        method: 'DELETE', headers: writeHeaders({ 'If-Match': quoteETag(updatedAt) }), signal,
      });
    },
    refreshSubscriptionSource(sourceID, signal) {
      return request<SubscriptionSource>(fetcher, `${baseUrl}/subscription/sources/${encodeURIComponent(sourceID)}/refresh`, {
        method: 'POST',
        headers: writeHeaders(),
        signal,
      });
    },
    previewSubscriptionSource(input, signal) {
      return request<SubscriptionSourcePreview>(fetcher, `${baseUrl}/subscription/sources/preview`, {
        method: 'POST', body: JSON.stringify(input), headers: writeJSONHeaders(), signal,
      });
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
