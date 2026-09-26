import type {
  CreateSubscriptionSourceVersionData,
  SubscriptionChannel,
  SubscriptionSource,
  SubscriptionSourceVersion,
} from '../generated';

export type {
  ChannelNativeTemplate,
  ChannelNodeSelection,
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
  SubscriptionNodeSummary,
  SubscriptionPreview,
  SubscriptionSource,
  SubscriptionSourcePage,
  SubscriptionSourceSummary,
  SubscriptionSourceVersion,
  SubscriptionSourceVersionPage,
  SubscriptionSourceVersionSave,
  SubscriptionSourceInput as SubscriptionSourceWrite,
  SubscriptionToken,
  SubscriptionTokenPage,
  SubscriptionTokenRotation,
  SubscriptionTokenSecret,
  SubscriptionUser,
  SubscriptionUserGrants,
  SubscriptionUserPage,
  SubscriptionUserInput as SubscriptionUserWrite,
} from '../generated';

export type SubscriptionFormat = SubscriptionChannel['format'];
export type SubscriptionSourceKind = SubscriptionSource['source_kind'];
export type SubscriptionSourceFormat = CreateSubscriptionSourceVersionData['body']['format'];
export type SubscriptionSourceVersionFormat = SubscriptionSourceVersion['format'];

export interface SubscriptionListFilter {
  limit?: number;
  beforeID?: string;
  beforeTime?: string;
}
