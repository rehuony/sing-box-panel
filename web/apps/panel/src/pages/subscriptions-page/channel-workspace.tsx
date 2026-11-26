import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@shared/components/ui/dropdown-menu';
import {
  ArrowLeft,
  CirclePlus,
  Eye,
  Gauge,
  Link2,
  MoreHorizontal,
  MousePointer2,
  Route,
  Save,
  Settings2,
  Shuffle,
} from 'lucide-react';

import type {
  ChannelNativeTemplate,
  ChannelRuleGroup,
  SubscriptionChannel,
  SubscriptionFormat,
  SubscriptionNode,
} from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { ErrorNotice } from '@/components/error-notice';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { StripAction, StripDivider } from '@/components/title-strip';
import { Rail, RailCount, RailHeading, RailItem } from '@/components/rail';

import type { ChannelGroupTab } from './channel-group-editor';

import { useExits } from './channel-exits';
import { ChannelOptions } from './channel-options';
import { ChannelPreview } from './channel-preview';
import { useChannelDraft } from './use-channel-draft';
import { ChannelClientTag } from './channel-client-tag';
import { ChannelGroupEditor } from './channel-group-editor';
import { ChannelLinkPopover } from './channel-link-popover';
import { ChannelGroupSettings } from './channel-group-settings';
import { ChannelTemplateEditor } from './channel-template-editor';
import { ChannelRouteAdd, ChannelRouteList } from './channel-route-list';
import { defaultGroupHealthCheck, emptyMatch, exitsThrough, groupTypeName, matchedNodes } from './channel-policy';

const groupIcons = { 'select': MousePointer2, 'url-test': Gauge, 'fallback': Shuffle };

/**
 * A strategy group in the rail: its strategy's icon, its name, marked when it
 * is the final exit, and its members, its strategy and the exit rules through
 * it; settings and a menu that sets or clears the final exit, moves or
 * deletes it take the count's place on hover.
 */
function GroupRailItem({
  busy, current, final, first, format, group, last, matched, onDelete, onFinal, onMove, onSelect, onSettings, routes,
}: {
  group: ChannelRuleGroup;
  format: SubscriptionFormat;
  /** The pool nodes its match takes besides its members. */
  matched: number;
  /** The exit rules that leave through it. */
  routes: number;
  current: boolean;
  /** Whether what no exit rule matches leaves through it. */
  final: boolean;
  first: boolean;
  last: boolean;
  busy: boolean;
  onSelect: () => void;
  onSettings: () => void;
  /** Sets or clears it as the final exit. */
  onFinal: () => void;
  onMove: (delta: number) => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const [menuOpen, setMenuOpen] = useState(false);
  const Icon = groupIcons[group.type];
  const name = group.name || t('channels.groupName');
  const sub = [
    t('channels.groupSub', { type: groupTypeName(group.type, format), count: routes }),
    group.enabled ? '' : t('channels.disabled'),
  ].filter(Boolean).join(' · ');
  return (
    <RailItem
      actions={(
        <>
          <IconAction aria-haspopup='dialog' disabled={busy} label={t('channels.groupSettings', { name })} onClick={onSettings} size='icon-xs'>
            <Settings2 aria-hidden='true' />
          </IconAction>
          <DropdownMenu onOpenChange={setMenuOpen} open={menuOpen}>
            {/* An open menu keeps its trigger in view after the pointer leaves the item. */}
            <DropdownMenuTrigger render={<IconAction className='data-popup-open:visible' disabled={busy} label={t('channels.groupMore', { name })} size='icon-xs' />}>
              <MoreHorizontal aria-hidden='true' />
            </DropdownMenuTrigger>
            <DropdownMenuContent align='end' className='w-auto min-w-32'>
              <DropdownMenuGroup>
                <DropdownMenuItem disabled={first} onClick={() => onMove(-1)}>{t('common.moveUp')}</DropdownMenuItem>
                <DropdownMenuItem disabled={last} onClick={() => onMove(1)}>{t('common.moveDown')}</DropdownMenuItem>
                <DropdownMenuItem onClick={onDelete} variant='destructive'>{t('common.delete')}</DropdownMenuItem>
                {/* A disabled group cannot become an exit; clearing stays available. */}
                <DropdownMenuItem disabled={!final && !group.enabled} onClick={onFinal}>
                  {t(final ? 'channels.clearFinalExit' : 'channels.setFinalExit')}
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      )}
      current={current}
      disabled={busy}
      end={menuOpen ? null : <RailCount>{group.members.length + matched}</RailCount>}
      icon={<Icon />}
      marker={final
        ? <span className='flex-none rounded-[var(--radius-pill)] border border-[var(--color-border-strong)] px-1.5 text-2xs leading-4 text-[var(--color-text-secondary)]'>{t('channels.finalExit')}</span>
        : null}
      name={name}
      onClick={onSelect}
      sub={sub}
    />
  );
}

interface Props {
  /** The key the last link used, and where a link made here reports its key. */
  linkKey: string;
  active?: boolean;
  onBack: () => void;
  /** Asks to delete the channel. */
  onDelete: () => void;
  /** The pool in its saved order. */
  nodes: SubscriptionNode[];
  channel: SubscriptionChannel;
  toolbarTarget?: HTMLElement | null;
  onLinkKeyChange: (id: string) => void;
  onSaved: (channel: SubscriptionChannel) => void;
}

/**
 * One channel: the rail of its exit rules and strategy groups beside the
 * one shown, with its name, client and the actions on it in the title strip.
 */
export function ChannelWorkspace({
  active = true, toolbarTarget, channel, linkKey, nodes, onBack, onDelete, onLinkKeyChange, onSaved,
}: Props) {
  const { t } = useTranslation();
  const [groupTab, setGroupTab] = useState<ChannelGroupTab>('nodes');
  const {
    config,
    setConfig,
    format,
    name,
    busy,
    ruleSets,
    optionsOpen,
    setOptionsOpen,
    templateOpen,
    setTemplateOpen,
    group,
    groupSettings,
    setGroupSettings,
    setGroupID,
    removeGroup,
    setRemoveGroup,
    requestRemoveGroup,
    confirmRemoveGroup,
    preview,
    previewError,
    setPreviewError,
    setPreview,
    dirty,
    conflict,
    confirmNavigation,
    render,
    save,
    showPreview,
    reorder,
    updateGroup,
    setRoutes,
    setDefaultExit,
    addGroup,
    applyOptions,
  } = useChannelDraft(channel, onSaved);
  const exits = useExits(config.groups, nodes);
  const firstGroup = config.groups.find(value => value.enabled);
  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col'>
      <ToolbarActions active={active} target={toolbarTarget}>
        {/* The channel keeps room for its name and client; when narrow, the actions take the next line. */}
        <div className='flex w-full min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1'>
          <div className='flex min-w-[min(100%,12rem)] flex-1 items-center gap-2'>
            <IconAction disabled={busy} label={t('channels.back')} onClick={() => confirmNavigation(onBack)} variant='flat'>
              <ArrowLeft aria-hidden='true' />
            </IconAction>
            <span className='min-w-0 truncate text-sm font-semibold' title={name}>{name}</span>
            <ChannelClientTag format={format} />
          </div>
          {/* Actions from looking to changing; saving stands apart after a divider. */}
          <div className='ms-auto flex flex-none items-center gap-2'>
            <ChannelLinkPopover
              channel={channel}
              keyID={linkKey}
              onKeyChange={onLinkKeyChange}
              trigger={<Button className='max-sm:w-(--control-height-sm) max-sm:px-0 data-popup-open:bg-[var(--color-hover)]' size='sm' type='button' variant='flat' />}
            >
              <Link2 aria-hidden='true' data-icon='inline-start' />
              <span className='max-sm:sr-only'>{t('channels.link')}</span>
            </ChannelLinkPopover>
            <StripAction disabled={busy} icon={<Eye aria-hidden='true' data-icon='inline-start' />} label={t('channels.preview')} onClick={() => void showPreview()} />
            <StripAction disabled={busy} icon={<Settings2 aria-hidden='true' data-icon='inline-start' />} label={t('channels.distribution')} onClick={() => setOptionsOpen(true)} />
            <StripDivider />
            <StripAction disabled={busy || conflict || !dirty} icon={<Save aria-hidden='true' data-icon='inline-start' />} label={t('channels.save')} onClick={() => void save()} />
          </div>
        </div>
      </ToolbarActions>
      {conflict && <ErrorNotice error={t('channels.formatConflict')} />}
      {/* The rail is a column of the channels card, divided by a hairline; on a phone a capped band above. */}
      <div className='grid min-h-0 flex-1 grid-cols-[16rem_minmax(0,1fr)] max-md:grid-cols-1 max-md:grid-rows-[minmax(0,13rem)_minmax(0,1fr)]'>
        <Rail className='rounded-none border-0 border-e bg-transparent max-md:border-e-0 max-md:border-b' label={t('channels.setup')}>
          <RailItem
            current={!group}
            disabled={busy}
            end={<RailCount>{config.routes.length}</RailCount>}
            icon={<Route />}
            name={t('channels.routes')}
            onClick={() => setGroupID(null)}
            sub={t('channels.finalExitSub', { exit: exits.label(config.default_exit) })}
          />
          <RailHeading end={(
            <IconAction disabled={busy} label={t('channels.addGroup')} onClick={addGroup} size='icon-xs'>
              <CirclePlus aria-hidden='true' />
            </IconAction>
          )}>
            {t('channels.groups')}
          </RailHeading>
          {config.groups.map((value, index) => (
            <GroupRailItem
              busy={busy}
              current={group?.id === value.id}
              final={exitsThrough(config.default_exit, value.id)}
              first={index === 0}
              format={format}
              group={value}
              key={value.id}
              last={index === config.groups.length - 1}
              matched={matchedNodes(value, nodes).length}
              onDelete={() => requestRemoveGroup(value.id)}
              onFinal={() => setDefaultExit(exitsThrough(config.default_exit, value.id) ? { kind: 'direct' } : { kind: 'group', id: value.id })}
              onMove={delta => reorder(index, delta)}
              onSelect={() => setGroupID(value.id)}
              onSettings={() => setGroupSettings(structuredClone(value))}
              routes={config.routes.filter(route => exitsThrough(route.exit, value.id)).length}
            />
          ))}
        </Rail>
        <div className='flex min-h-0 min-w-0 flex-col'>
          {group
            ? (
                <ChannelGroupEditor
                  busy={busy}
                  config={config}
                  format={format}
                  group={group}
                  key={group.id}
                  nodes={nodes}
                  onChange={updateGroup}
                  onRoutesChange={setRoutes}
                  onTabChange={setGroupTab}
                  ruleSets={ruleSets}
                  tab={groupTab}
                />
              )
            : (
                <section aria-label={t('channels.routes')} className='flex min-h-0 min-w-0 flex-1 flex-col'>
                  <div className='flex h-12 flex-none items-center gap-1 border-b ps-4 pe-3'>
                    <h2 className='min-w-0 flex-1 truncate text-sm font-medium'>{t('channels.routes')}</h2>
                    <ChannelRouteAdd
                      busy={busy}
                      exit={firstGroup ? { kind: 'group', id: firstGroup.id } : { kind: 'direct' }}
                      format={format}
                      onAdd={route => setRoutes([...config.routes, route])}
                      routes={config.routes}
                      ruleSets={ruleSets}
                    />
                  </div>
                  <ChannelRouteList
                    busy={busy}
                    config={config}
                    format={format}
                    nodes={nodes}
                    onRoutesChange={setRoutes}
                    ruleSets={ruleSets}
                  />
                </section>
              )}
        </div>
      </div>
      {groupSettings && (
        <ChannelGroupSettings
          busy={busy}
          format={format}
          nodes={nodes}
          onApply={(value) => {
            setConfig((current) => ({
              ...current,
              groups: current.groups.map((item) => item.id === value.id
                ? {
                    ...item,
                    name: value.name.trim(),
                    type: value.type,
                    health_check: value.type !== 'select' ? value.health_check ?? defaultGroupHealthCheck : undefined,
                    match: emptyMatch(value.match) ? undefined : value.match,
                    enabled: true,
                  }
                : item),
            }));
            setGroupSettings(null);
          }}
          onChange={setGroupSettings}
          onClose={() => setGroupSettings(null)}
          value={groupSettings}
        />
      )}
      {optionsOpen && (
        <ChannelOptions
          name={name}
          format={format}
          template={Boolean(config.template)}
          onClose={() => setOptionsOpen(false)}
          onDelete={onDelete}
          onSave={applyOptions}
          onTemplate={() => setTemplateOpen(true)}
        />
      )}
      {templateOpen && (
        <ChannelTemplateEditor
          template={config.template}
          format={format}
          nodes={nodes}
          onClose={() => setTemplateOpen(false)}
          onPreview={(template: ChannelNativeTemplate, signal) => render({ ...config, template }, signal)}
          onApply={(template) => setConfig((current) => ({ ...current, template }))}
        />
      )}
      {(preview || previewError != null) && (
        <ChannelPreview nodes={nodes} preview={preview} error={previewError} onClose={() => {
          setPreview(null);
          setPreviewError(null);
        }} />
      )}
      <Dialog open={removeGroup != null} onOpenChange={(open) => !open && setRemoveGroup(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('channels.deleteGroup')}</DialogTitle>
            <DialogDescription>
              {t('channels.deleteGroupPrompt', { name: config.groups.find((value) => value.id === removeGroup)?.name })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant='outline' onClick={() => setRemoveGroup(null)}>
              {t('common.cancel')}
            </Button>
            <Button variant='destructive' onClick={confirmRemoveGroup}>
              {t('channels.deleteGroup')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
