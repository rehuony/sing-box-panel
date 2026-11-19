import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { Field, FieldGroup, FieldLabel } from '@shared/components/ui/field';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';
import { ArrowDown, ArrowLeft, ArrowUp, CirclePlus, Eye, Flag, Gauge, ListRestart, MousePointer2, Save, Settings2, Trash2 } from 'lucide-react';

import type {
  ChannelNativeTemplate,
  ChannelRuleGroup,
  SubscriptionChannel,
  SubscriptionNode,
} from '@/api/api-client';

import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ToolbarActions } from '@/components/workspace-toolbar';

import type { ChannelGroupTab } from './channel-group-editor';

import { ChannelOptions } from './channel-options';
import { ChannelPreview } from './channel-preview';
import { useChannelDraft } from './use-channel-draft';
import { ChannelGroupEditor } from './channel-group-editor';
import { ChannelTemplateEditor } from './channel-template-editor';
import { SubscriptionContentState } from './subscription-content-state';
import { defaultGroupHealthCheck,
  nextRuleIndex, removeGroupReferences } from './channel-policy';

interface Props {
  active?: boolean;
  onBack: () => void;
  /** The pool in its saved order. */
  nodes: SubscriptionNode[];
  channel: SubscriptionChannel;
  toolbarTarget?: HTMLElement | null;
  onSaved: (channel: SubscriptionChannel) => void;
}
export function ChannelWorkspace({ active = true, toolbarTarget, channel, nodes, onBack, onSaved }: Props) {
  const { t } = useTranslation();
  const [groupTab, setGroupTab] = useState<ChannelGroupTab>('nodes');
  const {
    config,
    setConfig,
    format,
    name,
    busy,
    optionsOpen,
    setOptionsOpen,
    templateOpen,
    setTemplateOpen,
    group,
    groupIndex,
    finalGroup,
    groupSettings,
    setGroupSettings,
    setGroupID,
    removeGroup,
    setRemoveGroup,
    preview,
    previewError,
    setPreviewError,
    setPreview,
    dirty,
    conflict,
    ruleTextResults,
    setRuleView,
    changeRuleText,
    invalidTextGroup,
    confirmNavigation,
    render,
    save,
    showPreview,
    reorder,
    updateGroup,
    addGroup,
    applyOptions,
  } = useChannelDraft(channel, onSaved);
  return (
    <div className='@container/channel-editor flex h-full min-h-0 min-w-0 flex-col gap-2 px-4 pt-4 pb-2 sm:px-6 sm:pt-6 [&_[data-slot=button][data-variant=ghost]]:border-transparent'>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='ms-auto flex min-w-0 flex-none items-center justify-end gap-2'>
          <Button size='icon-sm' variant='flat' aria-label={t('channels.back')} title={t('channels.back')} disabled={busy} onClick={() => confirmNavigation(onBack)}><ArrowLeft /></Button>
          <Button size='icon-sm' variant='flat' aria-label={t('channels.distribution')} title={t('channels.distribution')} disabled={busy} onClick={() => setOptionsOpen(true)}><Settings2 /></Button>
          <Button size='icon-sm' variant='flat' aria-label={t('channels.preview')} title={t('channels.preview')} disabled={busy} onClick={() => void showPreview()}><Eye /></Button>
          <Button size='sm' variant='flat' disabled={busy || conflict || !dirty} onClick={() => {
            if (invalidTextGroup) {
              // Show the text that needs fixing; its last valid rules stay unsaved.
              setGroupID(invalidTextGroup.id);
              setGroupTab('rules');
              toast.add({ title: t('channels.ruleTextUnsaved', { name: invalidTextGroup.name }), type: 'error' });
              return;
            }
            void save();
          }}>
            <Save data-icon='inline-start' />
            {t('channels.save')}
          </Button>
        </div>
      </ToolbarActions>
      <div className='flex min-h-0 flex-1 flex-col gap-3 overflow-hidden @max-[740px]/channel-editor:overflow-y-auto @max-[740px]/channel-editor:pe-1'>
        {conflict && <ErrorNotice error={t('channels.formatConflict')} />}
        <div className='grid min-h-0 flex-1 grid-cols-[220px_minmax(0,1fr)] gap-4 @max-[740px]/channel-editor:flex @max-[740px]/channel-editor:flex-none @max-[740px]/channel-editor:flex-col @max-[740px]/channel-editor:has-[[data-rule-text]]:flex-[1_0_auto]'>
          <aside className='flex min-h-0 min-w-0 flex-col gap-4 overflow-hidden border-r pe-4 @max-[740px]/channel-editor:overflow-visible @max-[740px]/channel-editor:border-r-0 @max-[740px]/channel-editor:p-0' aria-label={t('channels.groups')}>
            <div className='flex flex-none flex-wrap items-center gap-2 [&>button]:ms-auto [&>h2]:text-sm [&>h2]:font-medium'>
              <h2>{t('channels.groups')}</h2>
              <Badge variant='secondary'>{config.groups.length}</Badge>
              <Button size='icon-sm' variant='ghost' aria-label={t('channels.addGroup')} title={t('channels.addGroup')} disabled={busy} onClick={addGroup}><CirclePlus /></Button>
            </div>
            <div className='-m-0.5 flex min-h-0 flex-1 flex-col gap-2.5 overflow-y-auto p-0.5 @max-[740px]/channel-editor:m-0 @max-[740px]/channel-editor:grid @max-[740px]/channel-editor:max-h-55 @max-[740px]/channel-editor:flex-none @max-[740px]/channel-editor:grid-cols-[repeat(auto-fill,minmax(200px,1fr))]'>
              {config.groups.map((value) => (
                <div className='group/card relative min-w-0 flex-none' key={value.id}>
                  <Button
                    variant='ghost'
                    className='min-h-16 w-full flex-col items-stretch gap-1 px-3 py-2.5 text-left font-normal whitespace-normal active:scale-100 aria-pressed:bg-accent aria-pressed:text-foreground'
                    data-group-item
                    size='content'
                    aria-pressed={group?.id === value.id}
                    disabled={busy}
                    onClick={() => setGroupID(value.id)}
                  >
                    <span className='flex items-center gap-2 pe-7 wrap-anywhere' title={`${value.name} · ${t(`channels.groupTypes.${value.type}`)}`}>
                      {value.type === 'url-test' ? <Gauge aria-hidden='true' /> : value.type === 'fallback' ? <ListRestart aria-hidden='true' /> : <MousePointer2 aria-hidden='true' />}
                      {value.name || t('channels.groupName')}
                    </span>
                    <span className='flex min-h-5 min-w-0 items-center gap-1.5'>
                      <span className='min-w-0 truncate text-xs text-muted-foreground' title={t('channels.groupSummary', { nodes: value.members.length, rules: value.rules.length })}>
                        {t('channels.groupSummary', { nodes: value.members.length, rules: value.rules.length })}
                      </span>
                      <span className='ms-auto flex flex-none items-center gap-1.5 empty:hidden *:data-[slot=badge]:font-normal'>
                        {!value.enabled && <Badge variant='secondary'>{t('channels.disabled')}</Badge>}
                        {config.default_exit.kind === 'group' && config.default_exit.id === value.id && <Badge variant='success'>{t('channels.finalExit')}</Badge>}
                      </span>
                    </span>
                  </Button>
                  <Button
                    size='icon-sm' variant='ghost' className='pointer-events-none absolute end-1 top-1 border-transparent bg-transparent opacity-0 transition-opacity duration-[var(--dur-short)] group-hover/card:pointer-events-auto group-hover/card:opacity-100 group-has-focus-visible/card:pointer-events-auto group-has-focus-visible/card:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100'
                    aria-label={t('channels.groupSettings', { name: value.name })}
                    title={t('channels.groupSettings', { name: value.name })}
                    aria-haspopup='dialog' disabled={busy}
                    onClick={() => setGroupSettings(structuredClone(value))}
                  >
                    <Settings2 />
                  </Button>
                </div>
              ))}
            </div>
            {group && (
              <div className='flex flex-none flex-wrap items-center gap-2 pt-1'>
                <Button size='icon-sm' variant='ghost' aria-label={t('channels.moveGroupUp')} title={t('channels.moveGroupUp')} disabled={busy || groupIndex === 0} onClick={() => reorder(groupIndex, -1)}><ArrowUp /></Button>
                <Button size='icon-sm' variant='ghost' aria-label={t('channels.moveGroupDown')} title={t('channels.moveGroupDown')} disabled={busy || groupIndex === config.groups.length - 1} onClick={() => reorder(groupIndex, 1)}><ArrowDown /></Button>
                <Button
                  size='icon-sm' variant='ghost' className='ms-auto'
                  aria-label={t(finalGroup ? 'channels.clearFinalExit' : 'channels.setFinalExit')}
                  title={t(finalGroup ? 'channels.clearFinalExit' : 'channels.setFinalExit')}
                  aria-pressed={finalGroup} disabled={busy}
                  onClick={() => setConfig((current) => ({
                    ...current,
                    groups: current.groups.map((item) =>
                      item.id === group.id ? { ...item, enabled: true } : item),
                    default_exit: current.default_exit.kind === 'group' && current.default_exit.id === group.id
                      ? { kind: 'direct' }
                      : { kind: 'group', id: group.id },
                  }))}
                >
                  <Flag fill={finalGroup ? 'currentColor' : 'none'} />
                </Button>
                <Button variant='ghost' size='icon-sm' className='text-[var(--color-danger)]' aria-label={t('channels.deleteGroup')} title={t('channels.deleteGroup')} disabled={busy} onClick={() => {
                  if (config.default_exit.kind === 'group' && config.default_exit.id === group.id) {
                    toast.add({ title: t('channels.groupExitInUse'), type: 'error' });
                    return;
                  }
                  setRemoveGroup(group.id);
                }}>
                  <Trash2 />
                </Button>
              </div>
            )}
          </aside>
          {group
            ? (
                <ChannelGroupEditor
                  key={group.id} group={group} groups={config.groups} nodes={nodes}
                  tab={groupTab} onTabChange={setGroupTab}
                  format={format} busy={busy} onChange={updateGroup} nextSortIndex={nextRuleIndex(config)}
                  ruleText={ruleTextResults.get(group.id)}
                  onRuleView={view => setRuleView(group, view)} onRuleTextChange={text => changeRuleText(group, text)}
                />
              )
            : (
                <SubscriptionContentState kind='groups' title={t('channels.noGroups')} />
              )}
        </div>
      </div>
      {groupSettings && (
        <Dialog open onOpenChange={(open) => !open && setGroupSettings(null)}>
          <DialogContent className='w-110 max-w-[calc(100vw-32px)] sm:max-w-110'>
            <DialogHeader>
              <DialogTitle>{t('channels.groupSettingsTitle')}</DialogTitle>
              <DialogDescription className='sr-only'>{groupSettings.name}</DialogDescription>
            </DialogHeader>
            <form className='flex flex-col gap-6' onSubmit={(event) => {
              event.preventDefault();
              if (busy || !groupSettings.name.trim()) return;
              setConfig((current) => ({
                ...current,
                groups: current.groups.map((item) => item.id === groupSettings.id
                  ? { ...item, name: groupSettings.name.trim(), type: groupSettings.type, health_check: groupSettings.type !== 'select' ? groupSettings.health_check ?? defaultGroupHealthCheck : undefined, enabled: true }
                  : item),
              }));
              setGroupSettings(null);
            }}>
              <FieldGroup className='gap-4 *:data-[slot=field]:grid *:data-[slot=field]:min-h-(--control-height-sm) *:data-[slot=field]:grid-cols-[112px_minmax(0,1fr)] *:data-[slot=field]:items-center *:data-[slot=field]:gap-4 [&_[data-slot=field-label]]:font-normal [&_[data-slot=input]]:w-full [&_[data-slot=input]]:min-w-0 [&_[data-slot=select-trigger]]:w-full [&_[data-slot=select-trigger]]:min-w-0'>
                <Field orientation='horizontal'>
                  <FieldLabel htmlFor='group-name'>{t('channels.groupName')}</FieldLabel>
                  <Input id='group-name' maxLength={128} disabled={busy} value={groupSettings.name} onChange={(event) => setGroupSettings({ ...groupSettings, name: event.target.value })} />
                </Field>
                <Field orientation='horizontal'>
                  <FieldLabel htmlFor='group-type'>{t('channels.groupType')}</FieldLabel>
                  <SelectField<NonNullable<ChannelRuleGroup['type']>>
                    id='group-type' value={groupSettings.type} disabled={busy}
                    items={(['select', 'url-test', 'fallback'] as const).map((type) => ({ value: type, label: t(`channels.groupTypes.${type}`), disabled: format === 'sing-box' && type === 'fallback' }))}
                    onValueChange={(type) => setGroupSettings({ ...groupSettings, type })}
                  />
                </Field>
                {groupSettings.type !== 'select' && (
                  <>
                    <Field orientation='horizontal'>
                      <FieldLabel htmlFor='group-test-url'>{t('channels.testURL')}</FieldLabel>
                      <Input id='group-test-url' type='url' required maxLength={4096} disabled={busy} value={(groupSettings.health_check ?? defaultGroupHealthCheck).url} onChange={(event) => setGroupSettings({ ...groupSettings, health_check: { ...(groupSettings.health_check ?? defaultGroupHealthCheck), url: event.target.value } })} />
                    </Field>
                    <Field orientation='horizontal'>
                      <FieldLabel htmlFor='group-test-interval'>{t('channels.testInterval')}</FieldLabel>
                      <Input id='group-test-interval' type='number' min={60} max={86400} required disabled={busy} value={(groupSettings.health_check ?? defaultGroupHealthCheck).interval} onChange={(event) => setGroupSettings({ ...groupSettings, health_check: { ...(groupSettings.health_check ?? defaultGroupHealthCheck), interval: Number(event.target.value) } })} />
                    </Field>
                    {groupSettings.type === 'url-test' && (
                      <Field orientation='horizontal'>
                        <FieldLabel htmlFor='group-test-tolerance'>{t('channels.testTolerance')}</FieldLabel>
                        <Input id='group-test-tolerance' type='number' min={0} max={65535} required disabled={busy} value={(groupSettings.health_check ?? defaultGroupHealthCheck).tolerance} onChange={(event) => setGroupSettings({ ...groupSettings, health_check: { ...(groupSettings.health_check ?? defaultGroupHealthCheck), tolerance: Number(event.target.value) } })} />
                      </Field>
                    )}
                  </>
                )}
              </FieldGroup>
              <DialogFooter>
                <Button type='button' variant='outline' onClick={() => setGroupSettings(null)}>{t('common.cancel')}</Button>
                <Button type='submit' disabled={busy || !groupSettings.name.trim()}>{t('channels.apply')}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}
      {optionsOpen && (
        <ChannelOptions
          name={name}
          format={format}
          template={Boolean(config.template)}
          onClose={() => setOptionsOpen(false)}
          onSave={applyOptions}
          onTemplate={() => setTemplateOpen(true)}
        />
      )}
      {templateOpen && (
        <ChannelTemplateEditor
          template={config.template}
          format={format}
          onClose={() => setTemplateOpen(false)}
          onPreview={(template: ChannelNativeTemplate, signal) => render({ ...config, template }, signal)}
          onApply={(template) => setConfig((current) => ({ ...current, template }))}
        />
      )}
      {(preview || previewError != null) && (
        <ChannelPreview preview={preview} error={previewError} onClose={() => {
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
            <Button
              variant='destructive'
              onClick={() => {
                setConfig({
                  ...config,
                  groups: removeGroupReferences(
                    config.groups.filter((value) => value.id !== removeGroup), removeGroup!,
                  ),
                });
                setRemoveGroup(null);
              }}
            >
              {t('channels.deleteGroup')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
