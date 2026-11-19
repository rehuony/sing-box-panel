import type {
  SubscriptionChannel,
} from '../generated';

export type {
  ChannelGroupMember,
  ChannelNativeTemplate,
  ChannelRemoteRuleSet,
  ChannelRouteExit,
  ChannelRule,
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
  SubscriptionSource,
  SubscriptionSourceContent,
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
