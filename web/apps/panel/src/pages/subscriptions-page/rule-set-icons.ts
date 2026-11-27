import type { LucideIcon } from 'lucide-react';

import {
  Apple,
  Ban,
  Bot,
  Cloud,
  Code,
  Film,
  Flag,
  Gamepad2,
  Globe,
  House,
  Landmark,
  Layers,
  Lock,
  Mail,
  MapPin,
  MessageCircle,
  Music,
  Newspaper,
  Send,
  Server,
  ShieldBan,
  ShieldCheck,
  ShoppingCart,
  Tv,
} from 'lucide-react';

/**
 * The icons a rule set can take, by the key the panel stores, in the order
 * the icon menu shows them: six a row, from general places to security,
 * content and everyday services. A key this list lacks draws the first.
 */
export const RULE_SET_ICONS = [
  ['layers', Layers],
  ['globe', Globe],
  ['map-pin', MapPin],
  ['flag', Flag],
  ['house', House],
  ['server', Server],
  ['cloud', Cloud],
  ['shield-check', ShieldCheck],
  ['shield-ban', ShieldBan],
  ['ban', Ban],
  ['lock', Lock],
  ['code', Code],
  ['bot', Bot],
  ['gamepad', Gamepad2],
  ['tv', Tv],
  ['film', Film],
  ['music', Music],
  ['newspaper', Newspaper],
  ['message', MessageCircle],
  ['send', Send],
  ['mail', Mail],
  ['cart', ShoppingCart],
  ['bank', Landmark],
  ['apple', Apple],
] as const satisfies readonly (readonly [string, LucideIcon])[];

export type RuleSetIconKey = (typeof RULE_SET_ICONS)[number][0];

export const DEFAULT_RULE_SET_ICON: RuleSetIconKey = 'layers';

/** The drawn icon of a stored key, or the default one for a key the menu does not offer. */
export function ruleSetIcon(key: string | undefined): { key: RuleSetIconKey; Icon: LucideIcon } {
  const found = RULE_SET_ICONS.find(([each]) => each === key) ?? RULE_SET_ICONS[0];
  return { key: found[0], Icon: found[1] };
}
