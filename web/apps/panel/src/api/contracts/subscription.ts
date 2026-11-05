import type {
  SubscriptionChannel,
} from '../generated';

export type {
  ChannelNativeTemplate,
  ChannelPolicy,
  ChannelRemoteRuleSet,
  ChannelRouteExit,
  ChannelRule,
  ChannelRuleGroup,
  CreatedSubscriptionToken,
  SubscriptionChannel,
  SubscriptionChannelConfig,
  SubscriptionChannelPage,
  SubscriptionChannelSummary,
  SubscriptionChannelInput as SubscriptionChannelWrite,
  SubscriptionCursor,
  SubscriptionDraftPreview,
  SubscriptionNodeCatalog,
  SubscriptionNodeDetail,
  SubscriptionNodeOrder,
  SubscriptionNodeSummary,
  SubscriptionPreview,
  SubscriptionSource,
  SubscriptionSourcePage,
  SubscriptionSourceSummary,
  SubscriptionSourceVersion,
  SubscriptionSourceInput as SubscriptionSourceWrite,
  SubscriptionToken,
  SubscriptionTokenPage,
  SubscriptionTokenRotation,
  SubscriptionUser,
  SubscriptionUserPage,
} from '../generated';

export type SubscriptionFormat = SubscriptionChannel['format'];

export interface SubscriptionListFilter {
  limit?: number;
  beforeID?: string;
  beforeTime?: string;
}
