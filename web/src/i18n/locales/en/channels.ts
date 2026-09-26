export const channels = {
  legacyUpgradeHint: 'This channel still delivers nodes only. Both previews show the full configuration that Save changes will publish. Existing filters become exclusions for current nodes; future nodes follow the new-node policy.',
  sortIndex: 'Sort index',
  sortIndexHint: 'Lower numbers come first. Repeated indices are ordered by name.',
  previewIssues: 'Issues ({{count}})',
  previewUnavailable: 'Configuration could not be generated. Open the issues beside the title for details.',

  templateDraftHint: 'Apply updates the channel draft. Save changes in the channel to publish it.',
  applyTemplate: 'Apply',
  replaceTemplateHint: 'Switching clients replaces the current template with the target client’s defaults. Review incompatible rules before saving.',
  switchTemplate: 'Switch and replace template',
  loonRuleOrder: 'Loon prioritizes local rules over remote rules and domain matches over IP matches. FINAL handles unmatched traffic.',

  selectedNodes: '{{count}} selected', clearNodes: 'Clear selection', removeNodes: 'Remove selected nodes',
  nodeDragInstructions: 'Press Space or Enter to select. Press F2 to start reordering, arrow keys to move, F2 to finish, or Escape to cancel.',
  nodeDragStart: 'Moving {{name}}.', nodeDragPosition: 'Position {{position}} of {{count}}.', nodeDragEnd: 'Node order updated.', nodeDragCancel: 'Reordering canceled.',
  noActiveKeys: 'No active keys. Create or enable a key in Key management.', keyUnavailable: 'This key is currently unavailable.', link: 'Link', duplicateSuffix: 'copy', duplicated: 'Channel duplicated',

  addNodes: 'Add nodes', addSelectedNodes: 'Add {{count}} nodes', pickNodesHint: 'Choose nodes to add to this group.',
  emptyCandidates: 'No candidate nodes yet', emptyCandidatesHint: 'Use the plus button above to add nodes to this group.',

  groupType: 'Group type', groupTypes: { 'select': 'Manual selection', 'url-test': 'Automatic latency test', 'fallback': 'Fallback' },
  testURL: 'Test link', testInterval: 'Interval (s)', testTolerance: 'Tolerance (ms)',
  addBuiltin: 'Add built-in node', builtinNode: 'Built-in node', mihomoOnly: 'Mihomo only',
  rejectSupport: 'REJECT candidates are supported by Mihomo and Loon manual groups.',
  singBoxGroupSupport: 'sing-box supports manual selection and automatic latency tests.',

  accelerationUnavailable: 'Enter a GitHub file link to enable acceleration.',
  groupSettings: 'Configure {{name}}', groupSettingsTitle: 'Strategy group settings', clearFinalExit: 'Clear final exit',
  setFinalExit: 'Set as final exit', finalExitHint: 'Use this group for unmatched traffic. Selecting a final group is optional.',
  applyClientFirst: 'Choose Done to apply the output client before editing its template.',
  members: 'Group members', membersCount: '{{count}} nodes', finalExit: 'Final exit', searchNodes: 'Search nodes, sources or protocols', noMatchingNodes: 'No matching nodes', searchNodesHint: 'Try another name, source or protocol.', ruleSet: 'Rule set',
  newGroupName: 'Strategy group {{number}}', groupSummary: '{{nodes}} nodes · {{rules}} rules', noGroups: 'No strategy groups yet', noGroupsHint: 'Add a group on the left, then choose its nodes and traffic exits.', noNodes: 'No nodes available. Add nodes in Subscription sources.', noRules: 'Matching traffic uses this group’s nodes. Rules are evaluated in order.', groupExitInUse: 'This group is the final exit. Clear its final-exit selection or choose another group before deleting or disabling it.', moveGroupUp: 'Move group up', moveGroupDown: 'Move group down',
  subscriptionKey: 'Subscription key', keyPlaceholder: 'Paste an existing key', keyHint: 'Create keys in Key management. The same key can be used with multiple channels; it is not saved here.', copyURL: 'Copy subscription URL', channelUnavailable: 'This channel is disabled and cannot distribute subscriptions.',
  disabled: 'Disabled',
  title: 'Channels', search: 'Search channels', add: 'Add channel', name: 'Channel name', client: 'Output client', edit: 'Edit', remove: 'Delete channel', deletePrompt: 'Delete “{{name}}”? Its subscription URL will become unavailable.', save: 'Save changes', saved: 'Channel saved', empty: 'No channels', back: 'Back', distribution: 'Channel settings', preview: 'Subscription preview', enabled: 'Enabled', nameRequired: 'Enter a name', cancel: 'Cancel', done: 'Done', refresh: 'Refresh nodes',

  newNodes: 'New-node policy', include: 'Include automatically', exclude: 'Select manually', template: 'Current template', defaultTemplate: 'Default configuration', customTemplate: 'Custom configuration', editTemplate: 'Edit template', templateCode: 'Native configuration', validate: 'Validate', valid: 'Template structure is valid', formatConflict: 'Output client changed. Review strategy types, built-in nodes, rule-set formats and template.', copy: 'Copy', copied: 'Copied',
  groups: 'Strategy groups', addGroup: 'Add strategy group', editGroup: 'Edit strategy group', groupName: 'Group name', matches: 'Exit rules', manualCount: '{{count}} rules', remoteCount: '{{count}} rule sets', state: 'State', actions: 'Actions', candidates: 'Candidate nodes', direct: 'Direct', reject: 'Reject', follow: 'Group nodes', missingNode: 'Unavailable node', deleteGroup: 'Delete strategy group', deleteRule: 'Delete rule',
  addRule: 'Add rule', addRemote: 'Add rule set', editRule: 'Edit rule', editRemote: 'Edit rule set', ruleKind: 'Match type', domain: 'Domain', domain_suffix: 'Domain suffix', domain_keyword: 'Domain keyword', ip_cidr: 'IP / CIDR', matchValue: 'Match value', exit: 'Traffic exit', ruleName: 'Name', url: 'Link', acceleration: 'Accelerate GitHub file', sourceFormat: 'Source format', chooseFormat: 'Choose format', behavior: 'Rule behavior', interval: 'Update interval (seconds)', selectAll: 'Select all', unavailable: 'Currently unavailable', invalidRule: 'Check the match value, source URL or update interval.', formatPending: 'Choose a compatible format',
  publicHost: 'Existing channel public host', enabledLabel: 'Channel enabled',
} as const;
