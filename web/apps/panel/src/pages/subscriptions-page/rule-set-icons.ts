import type { TablerIcon } from '@tabler/icons-react';

import {
  IconBan,
  IconBook,
  IconBrandAlipay,
  IconBrandAmazon,
  IconBrandAndroid,
  IconBrandApple,
  IconBrandBaidu,
  IconBrandBilibili,
  IconBrandBinance,
  IconBrandBing,
  IconBrandCloudflare,
  IconBrandDingtalk,
  IconBrandDiscord,
  IconBrandDisney,
  IconBrandDocker,
  IconBrandDouban,
  IconBrandDropbox,
  IconBrandFacebook,
  IconBrandGithub,
  IconBrandGoogle,
  IconBrandHbo,
  IconBrandInstagram,
  IconBrandLine,
  IconBrandLinkedin,
  IconBrandNeteaseMusic,
  IconBrandNetflix,
  IconBrandOnedrive,
  IconBrandOpenai,
  IconBrandPaypal,
  IconBrandQq,
  IconBrandReddit,
  IconBrandSignal,
  IconBrandSpeedtest,
  IconBrandSpotify,
  IconBrandSteam,
  IconBrandTaobao,
  IconBrandTelegram,
  IconBrandTiktok,
  IconBrandTwitch,
  IconBrandWechat,
  IconBrandWeibo,
  IconBrandWhatsapp,
  IconBrandWikipedia,
  IconBrandWindows,
  IconBrandX,
  IconBrandXbox,
  IconBrandYoutube,
  IconBrandZhihu,
  IconBrandZoom,
  IconBriefcase,
  IconBuildingBank,
  IconCloud,
  IconCode,
  IconCurrencyBitcoin,
  IconDatabase,
  IconDeviceGamepad2,
  IconDeviceNintendo,
  IconDeviceTv,
  IconDownload,
  IconEyeOff,
  IconFlag,
  IconHeadphones,
  IconHome,
  IconKey,
  IconLock,
  IconMail,
  IconMapPin,
  IconMessageCircle,
  IconMovie,
  IconMusic,
  IconNews,
  IconPackage,
  IconPhoto,
  IconPlane,
  IconRefresh,
  IconRobot,
  IconRocket,
  IconSchool,
  IconSend,
  IconServer,
  IconShieldCheck,
  IconShieldX,
  IconShoppingCart,
  IconStack2,
  IconTerminal2,
  IconWallet,
  IconWifi,
  IconWorld,
} from '@tabler/icons-react';

type IconList = readonly (readonly [string, TablerIcon])[];

const GENERAL = [
  ['layers', IconStack2], ['globe', IconWorld], ['map-pin', IconMapPin], ['flag', IconFlag],
  ['house', IconHome], ['server', IconServer], ['cloud', IconCloud], ['wifi', IconWifi],
  ['rocket', IconRocket], ['download', IconDownload], ['refresh', IconRefresh], ['package', IconPackage],
  ['shield-check', IconShieldCheck], ['shield-ban', IconShieldX], ['ban', IconBan], ['eye-off', IconEyeOff],
  ['lock', IconLock], ['key', IconKey], ['code', IconCode], ['terminal', IconTerminal2],
  ['database', IconDatabase], ['briefcase', IconBriefcase], ['school', IconSchool], ['bot', IconRobot],
  ['gamepad', IconDeviceGamepad2], ['tv', IconDeviceTv], ['film', IconMovie], ['music', IconMusic],
  ['headphones', IconHeadphones], ['newspaper', IconNews], ['book', IconBook], ['image', IconPhoto],
  ['message', IconMessageCircle], ['send', IconSend], ['mail', IconMail], ['cart', IconShoppingCart],
  ['bank', IconBuildingBank], ['wallet', IconWallet], ['bitcoin', IconCurrencyBitcoin], ['plane', IconPlane],
] as const satisfies IconList;

const SERVICES = [
  ['apple', IconBrandApple], ['google', IconBrandGoogle], ['microsoft', IconBrandWindows], ['github', IconBrandGithub],
  ['openai', IconBrandOpenai], ['cloudflare', IconBrandCloudflare], ['android', IconBrandAndroid], ['amazon', IconBrandAmazon],
  ['telegram', IconBrandTelegram], ['whatsapp', IconBrandWhatsapp], ['discord', IconBrandDiscord], ['signal', IconBrandSignal],
  ['line', IconBrandLine], ['x', IconBrandX], ['facebook', IconBrandFacebook], ['instagram', IconBrandInstagram],
  ['youtube', IconBrandYoutube], ['netflix', IconBrandNetflix], ['disney', IconBrandDisney], ['hbo', IconBrandHbo],
  ['twitch', IconBrandTwitch], ['spotify', IconBrandSpotify], ['tiktok', IconBrandTiktok], ['reddit', IconBrandReddit],
  ['steam', IconBrandSteam], ['xbox', IconBrandXbox], ['nintendo', IconDeviceNintendo], ['paypal', IconBrandPaypal],
  ['binance', IconBrandBinance], ['speedtest', IconBrandSpeedtest], ['wikipedia', IconBrandWikipedia], ['linkedin', IconBrandLinkedin],
  ['bilibili', IconBrandBilibili], ['wechat', IconBrandWechat], ['qq', IconBrandQq], ['alipay', IconBrandAlipay],
  ['taobao', IconBrandTaobao], ['weibo', IconBrandWeibo], ['zhihu', IconBrandZhihu], ['douban', IconBrandDouban],
  ['baidu', IconBrandBaidu], ['netease-music', IconBrandNeteaseMusic], ['dingtalk', IconBrandDingtalk], ['onedrive', IconBrandOnedrive],
  ['dropbox', IconBrandDropbox], ['docker', IconBrandDocker], ['zoom', IconBrandZoom], ['bing', IconBrandBing],
] as const satisfies IconList;

/**
 * The icons a rule set can take, by the key the panel stores, in the icon
 * menu's two sections: general marks, then the services rule sets are
 * commonly named after. Each section fills whole rows of eight, so the
 * arrow keys move straight up and down across them.
 */
export const RULE_SET_ICON_SECTIONS = [['general', GENERAL], ['services', SERVICES]] as const;

export const RULE_SET_ICONS = [...GENERAL, ...SERVICES] as const;

export type RuleSetIconKey = (typeof RULE_SET_ICONS)[number][0];

/** The key a new rule set takes, which is also the server's default. */
export const DEFAULT_RULE_SET_ICON: RuleSetIconKey = 'layers';

/** The drawn icon of a stored key, or the default one for a key the menu does not offer. */
export function ruleSetIcon(key: string | undefined): { key: RuleSetIconKey; Icon: TablerIcon } {
  const found = RULE_SET_ICONS.find(([each]) => each === key) ?? RULE_SET_ICONS[0];
  return { key: found[0], Icon: found[1] };
}
