import type { TFunction } from 'i18next';
import type { ComponentProps } from 'react';

import { CSS } from '@dnd-kit/utilities';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
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
  ChannelRouteExit,
  ChannelRuleGroup,
  SubscriptionChannel,
  SubscriptionFormat,
  SubscriptionNode,
} from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { ErrorNotice } from '@/components/error-notice';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { StripAction, StripDivider } from '@/components/title-strip';
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';
import { Rail, RailCount, RailHeading, RailItem, RailItemOverlay } from '@/components/rail';

import type { ChannelGroupTab } from './channel-group-editor';

import { dropSlot } from './drag-slot';
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
import { SubscriptionNodeSortContext } from './subscription-node-sort-context';
import { defaultGroupHealthCheck, emptyMatch, exitsThrough, groupTypeName, matchedNodes } from './channel-policy';

const groupIcons = { 'select': MousePointer2, 'url-test': Gauge, 'fallback': Shuffle };

type RailFace = Pick<ComponentProps<typeof RailItemOverlay>, 'end' | 'icon' | 'marker' | 'name' | 'sub'>;

/**
 * What a strategy group's rail item shows, the same on the copy a drag
 * carries: its strategy's icon, its name, marked when it is the final exit,
 * its strategy and the exit rules through it, and its members.
 */
function groupFace(t: TFunction, group: ChannelRuleGroup, format: SubscriptionFormat, { final, members, routes }: {
  /** Whether what no exit rule matches leaves through it. */
  final: boolean;
  /** Its members and the pool nodes its match takes. */
  members: number;
  /** The exit rules that leave through it. */
  routes: number;
}): RailFace {
  const Icon = groupIcons[group.type];
  return {
    end: <RailCount>{members}</RailCount>,
    icon: <Icon />,
    marker: final
      ? <span className='flex-none rounded-[var(--radius-pill)] border border-[var(--color-border-strong)] px-1.5 text-2xs leading-4 text-[var(--color-text-secondary)]'>{t('channels.finalExit')}</span>
      : undefined,
    name: group.name || t('channels.groupName'),
    sub: [
      t('channels.groupSub', { type: groupTypeName(group.type, format), count: routes }),
      group.enabled ? '' : t('channels.disabled'),
    ].filter(Boolean).join(' · '),
  };
}

/**
 * A strategy group in the rail, which drags into its place among the groups
 * from anywhere on it with the mouse or a touch, or with F2 from the
 * keyboard. Its settings and a menu that sets or clears the final exit or
 * deletes it take the count's place on hover.
 */
function GroupRailItem({ busy, current, face, final, group, onDelete, onFinal, onSelect, onSettings }: {
  group: ChannelRuleGroup;
  face: RailFace;
  current: boolean;
  /** Whether what no exit rule matches leaves through it. */
  final: boolean;
  busy: boolean;
  onSelect: () => void;
  onSettings: () => void;
  /** Sets or clears it as the final exit. */
  onFinal: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const reducedMotion = usePrefersReducedMotion();
  const [menuOpen, setMenuOpen] = useState(false);
  const { attributes, isDragging, listeners, setNodeRef, transform, transition }
    = useSortable({ id: group.id, disabled: busy });
  return (
    <RailItem
      {...attributes}
      {...listeners}
      {...face}
      actions={(
        <>
          <IconAction aria-haspopup='dialog' disabled={busy} label={t('channels.groupSettings', { name: face.name })} onClick={onSettings} size='icon-xs'>
            <Settings2 aria-hidden='true' />
          </IconAction>
          <DropdownMenu onOpenChange={setMenuOpen} open={menuOpen}>
            {/* An open menu keeps its trigger in view after the pointer leaves the item. */}
            <DropdownMenuTrigger render={<IconAction className='data-popup-open:visible' disabled={busy} label={t('channels.groupMore', { name: face.name })} size='icon-xs' />}>
              <MoreHorizontal aria-hidden='true' />
            </DropdownMenuTrigger>
            <DropdownMenuContent align='end' className='w-auto min-w-32'>
              <DropdownMenuGroup>
                {/* A disabled group cannot become an exit; clearing stays available. */}
                <DropdownMenuItem disabled={!final && !group.enabled} onClick={onFinal}>
                  {t(final ? 'channels.clearFinalExit' : 'channels.setFinalExit')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={onDelete} variant='destructive'>{t('common.delete')}</DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      )}
      aria-disabled={undefined}
      aria-keyshortcuts={busy ? undefined : 'F2'}
      aria-roledescription={t('channels.sortableGroup')}
      className='touch-pan-y select-none'
      current={current}
      disabled={busy}
      end={menuOpen ? null : face.end}
      item={{
        className: isDragging ? dropSlot : undefined,
        ref: setNodeRef,
        style: { transform: CSS.Translate.toString(transform), transition: reducedMotion ? undefined : transition },
      }}
      onClick={onSelect}
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
    render,
    save,
    showPreview,
    moveGroup,
    updateGroup,
    setRoutes,
    setDefaultExit,
    addGroup,
    applyOptions,
  } = useChannelDraft(channel, onSaved);
  const exits = useExits(config.groups, nodes);
  const draggingRef = useRef(false);
  const suppressClickRef = useRef(false);
  const groupFaces = config.groups.map((value) => {
    const final = exitsThrough(config.default_exit, value.id);
    const face = groupFace(t, value, format, {
      final,
      members: value.members.length + matchedNodes(value, nodes).length,
      routes: config.routes.filter(route => exitsThrough(route.exit, value.id)).length,
    });
    return { face, final, group: value };
  });
  const firstGroup = config.groups.find(value => value.enabled);
  // New exit rules go to the first enabled group, or out directly without one.
  const newExit: ChannelRouteExit = firstGroup ? { kind: 'group', id: firstGroup.id } : { kind: 'direct' };
  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col'>
      <ToolbarActions active={active} target={toolbarTarget}>
        {/* The channel keeps room for its name and client; when narrow, the actions take the next line. */}
        <div className='flex w-full min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1'>
          <div className='flex min-w-[min(100%,12rem)] flex-1 items-center gap-2'>
            <IconAction disabled={busy} label={t('channels.back')} onClick={onBack} variant='flat'>
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
        {/* A drag ends in a click on the group it started from; that click is not the user's. */}
        <div
          className='contents'
          onClickCapture={(event) => {
            if (!suppressClickRef.current) return;
            event.preventDefault();
            event.stopPropagation();
          }}
          onKeyDownCapture={(event) => {
            if (!draggingRef.current && event.key !== 'F2') suppressClickRef.current = false;
          }}
          onPointerDownCapture={() => {
            if (!draggingRef.current) suppressClickRef.current = false;
          }}
        >
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
            <SubscriptionNodeSortContext
              ended={t('channels.groupDragEnd')}
              instructions={t('channels.groupDragInstructions')}
              items={groupFaces.map(({ face, group: value }) => ({ id: value.id, label: face.name }))}
              onDraggingChange={(dragging) => {
                draggingRef.current = dragging;
                if (dragging) suppressClickRef.current = true;
              }}
              onMove={moveGroup}
              renderOverlay={(id) => {
                const dragged = groupFaces.find(({ group: value }) => value.id === id);
                return dragged ? <RailItemOverlay {...dragged.face} wideEnd /> : null;
              }}
              strategy={verticalListSortingStrategy}
            >
              {groupFaces.map(({ face, final, group: value }) => (
                <GroupRailItem
                  busy={busy}
                  current={group?.id === value.id}
                  face={face}
                  final={final}
                  group={value}
                  key={value.id}
                  onDelete={() => requestRemoveGroup(value.id)}
                  onFinal={() => setDefaultExit(final ? { kind: 'direct' } : { kind: 'group', id: value.id })}
                  onSelect={() => setGroupID(value.id)}
                  onSettings={() => setGroupSettings(structuredClone(value))}
                />
              ))}
            </SubscriptionNodeSortContext>
          </Rail>
        </div>
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
                      exit={newExit}
                      exitName={exits.label(newExit)}
                      format={format}
                      onAdd={added => setRoutes([...config.routes, ...added])}
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
