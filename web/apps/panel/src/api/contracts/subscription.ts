import type {
  SubscriptionChannel,
} from '../generated';

export type {
  ChannelGroupMatch,
  ChannelGroupMember,
  ChannelNativeTemplate,
  ChannelRemoteRuleSet,
  ChannelRoute,
  ChannelRouteExit,
  ChannelRuleGroup,
  CreatedSubscriptionToken,
  NodePublication,
  SubscriptionChannel,
  SubscriptionChannelConfig,
  SubscriptionChannelPage,
  SubscriptionChannelSummary,
  SubscriptionChannelInput as SubscriptionChannelWrite,
  SubscriptionCursor,
  SubscriptionDiagnostic,
  SubscriptionDraftPreview,
  SubscriptionNode,
  SubscriptionNodeDetail,
  SubscriptionNodeList,
  SubscriptionNodeUpdate,
  SubscriptionPreview,
  SubscriptionRule,
  SubscriptionRuleSet,
  SubscriptionRuleSetCreate,
  SubscriptionRuleSetList,
  SubscriptionRuleSetUpdate,
  SubscriptionSource,
  SubscriptionSourceCreate,
  SubscriptionSourceFilter,
  SubscriptionSourceFormatChoice,
  SubscriptionSourceKind,
  SubscriptionSourcePage,
  SubscriptionSourceParseError,
  SubscriptionSourcePreview,
  SubscriptionSourcePreviewItem,
  SubscriptionSourcePreviewRequest,
  SubscriptionSourceUpdate,
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
