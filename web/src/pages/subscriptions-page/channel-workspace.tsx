import { useTranslation } from 'react-i18next';
import { ArrowDown, ArrowLeft, ArrowUp, CirclePlus, Eye, Flag, Gauge, Layers3, ListRestart, MousePointer2, Save, Settings2, Trash2 } from 'lucide-react';

import type {
  ChannelNativeTemplate,
  ChannelRuleGroup,
  SubscriptionChannel,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { ChannelOptions } from './channel-options';
import { ChannelPreview } from './channel-preview';
import { useChannelDraft } from './use-channel-draft';
import { ChannelGroupEditor } from './channel-group-editor';
import {
  defaultGroupHealthCheck,
} from './channel-policy';
import { ChannelTemplateEditor } from './channel-template-editor';

interface Props {
  active?: boolean;
  onBack: () => void;
  channel: SubscriptionChannel;
  nodes: SubscriptionNodeSummary[];
  toolbarTarget?: HTMLElement | null;
  onSaved: (channel: SubscriptionChannel) => void;
}
export function ChannelWorkspace({ active = true, toolbarTarget, channel, nodes, onBack, onSaved }: Props) {
  const { t } = useTranslation();
  const {
    policy,
    setPolicy,
    config,
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
    setPreview,
    dirty,
    conflict,
    confirmNavigation,
    render,
    save,
    showPreview,
    reorder,
    updateGroup,
    addGroup,
    applyOptions,
  } = useChannelDraft(channel, nodes, onSaved);
  return (
    <div className='channel-workspace'>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='channel-workspace-actions workspace-toolbar-content'>
          <Button size='icon-sm' variant='ghost' aria-label={t('channels.back')} title={t('channels.back')} disabled={busy} onClick={() => confirmNavigation(onBack)}><ArrowLeft /></Button>
          <Button size='icon-sm' variant='ghost' aria-label={t('channels.distribution')} title={t('channels.distribution')} disabled={busy} onClick={() => setOptionsOpen(true)}><Settings2 /></Button>
          <Button size='icon-sm' variant='ghost' aria-label={t('channels.preview')} title={t('channels.preview')} disabled={busy} onClick={() => void showPreview()}><Eye /></Button>
          <Button size='sm' disabled={busy || conflict || !dirty} onClick={() => void save()}>
            <Save data-icon='inline-start' />
            {t('channels.save')}
          </Button>
        </div>
      </ToolbarActions>
      <div className='channel-rules-workspace'>
        {conflict && <ErrorNotice error={t('channels.formatConflict')} />}
        <div className='channel-policy-layout'>
          <aside className='channel-group-sidebar' aria-label={t('channels.groups')}>
            <div className='channel-group-sidebar-heading'>
              <h2>{t('channels.groups')}</h2>
              <Badge variant='secondary'>{policy.groups.length}</Badge>
              <Button size='icon-sm' variant='ghost' aria-label={t('channels.addGroup')} title={t('channels.addGroup')} disabled={busy} onClick={addGroup}><CirclePlus /></Button>
            </div>
            <div className='channel-group-list'>
              {policy.groups.map((value) => (
                <div className='channel-group-card' key={value.id}>
                  <Button
                    variant='ghost'
                    className='channel-group-item'
                    size='content'
                    aria-pressed={group?.id === value.id}
                    disabled={busy}
                    onClick={() => setGroupID(value.id)}
                  >
                    <span className='channel-group-item-name' title={`${value.name} · ${t(`channels.groupTypes.${value.type}`)}`}>
                      {value.type === 'url-test' ? <Gauge aria-hidden='true' /> : value.type === 'fallback' ? <ListRestart aria-hidden='true' /> : <MousePointer2 aria-hidden='true' />}
                      {value.name || t('channels.groupName')}
                    </span>
                    <span className='channel-group-metadata'>
                      <span className='channel-group-summary' title={t('channels.groupSummary', { nodes: value.node_ids.length + value.builtin_nodes.length, rules: value.rules.length })}>
                        {t('channels.groupSummary', { nodes: value.node_ids.length + value.builtin_nodes.length, rules: value.rules.length })}
                      </span>
                      <span className='channel-group-badges'>
                        {!value.enabled && <Badge variant='secondary'>{t('channels.disabled')}</Badge>}
                        {policy.default_exit.kind === 'group' && policy.default_exit.id === value.id && <Badge variant='success'>{t('channels.finalExit')}</Badge>}
                      </span>
                    </span>
                  </Button>
                  <Button
                    size='icon-sm' variant='ghost' className='channel-group-settings'
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
              <div className='channel-group-actions'>
                <Button size='icon-sm' variant='ghost' aria-label={t('channels.moveGroupUp')} title={t('channels.moveGroupUp')} disabled={busy || groupIndex === 0} onClick={() => reorder(groupIndex, -1)}><ArrowUp /></Button>
                <Button size='icon-sm' variant='ghost' aria-label={t('channels.moveGroupDown')} title={t('channels.moveGroupDown')} disabled={busy || groupIndex === policy.groups.length - 1} onClick={() => reorder(groupIndex, 1)}><ArrowDown /></Button>
                <Button
                  size='icon-sm' variant='ghost' className='channel-group-final-exit'
                  aria-label={t(finalGroup ? 'channels.clearFinalExit' : 'channels.setFinalExit')}
                  title={t(finalGroup ? 'channels.clearFinalExit' : 'channels.setFinalExit')}
                  aria-pressed={finalGroup} disabled={busy}
                  onClick={() => setPolicy((current) => ({
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
                <Button variant='ghost' size='icon-sm' className='channel-delete-action' aria-label={t('channels.deleteGroup')} title={t('channels.deleteGroup')} disabled={busy} onClick={() => {
                  if (policy.default_exit.kind === 'group' && policy.default_exit.id === group.id) {
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
                  key={group.id} group={group} nodes={nodes}
                  format={format} busy={busy} onChange={updateGroup}
                />
              )
            : (
                <Empty className='channel-group-empty'>
                  <EmptyHeader>
                    <EmptyMedia variant='icon'><Layers3 /></EmptyMedia>
                    <EmptyTitle>{t('channels.noGroups')}</EmptyTitle>
                    <EmptyDescription>{t('channels.noGroupsHint')}</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              )}
        </div>
      </div>
      {groupSettings && (
        <Dialog open onOpenChange={(open) => !open && setGroupSettings(null)}>
          <DialogContent className='channel-group-settings-dialog'>
            <DialogHeader>
              <DialogTitle>{t('channels.groupSettingsTitle')}</DialogTitle>
              <DialogDescription className='sr-only'>{groupSettings.name}</DialogDescription>
            </DialogHeader>
            <form className='channel-group-settings-form' onSubmit={(event) => {
              event.preventDefault();
              if (busy || !groupSettings.name.trim()) return;
              setPolicy((current) => ({
                ...current,
                groups: current.groups.map((item) => item.id === groupSettings.id
                  ? { ...item, name: groupSettings.name.trim(), type: groupSettings.type, health_check: groupSettings.type !== 'select' ? groupSettings.health_check ?? defaultGroupHealthCheck : undefined, enabled: true }
                  : item),
              }));
              setGroupSettings(null);
            }}>
              <FieldGroup className='channel-group-settings-fields'>
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
                <Button type='submit' disabled={busy || !groupSettings.name.trim()}>{t('channels.done')}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}
      {optionsOpen && (
        <ChannelOptions
          name={name}
          format={format}
          policy={policy}
          config={config}
          onClose={() => setOptionsOpen(false)}
          onSave={applyOptions}
          onTemplate={() => setTemplateOpen(true)}
        />
      )}
      {templateOpen && (
        <ChannelTemplateEditor
          template={policy.template}
          format={format}
          onClose={() => setTemplateOpen(false)}
          onPreview={(template: ChannelNativeTemplate, signal) => render({ ...policy, template }, signal)}
          onApply={(template) => setPolicy((current) => ({ ...current, template }))}
        />
      )}
      {preview && <ChannelPreview preview={preview} onClose={() => setPreview(null)} />}
      <Dialog open={removeGroup != null} onOpenChange={(open) => !open && setRemoveGroup(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('channels.deleteGroup')}</DialogTitle>
            <DialogDescription>
              {policy.groups.find((value) => value.id === removeGroup)?.name}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant='outline' onClick={() => setRemoveGroup(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={() => {
                setPolicy({
                  ...policy,
                  groups: policy.groups.filter((value) => value.id !== removeGroup),
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
