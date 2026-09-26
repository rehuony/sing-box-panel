import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, CirclePlus, RefreshCw, Search } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { SubscriptionNodeGrid } from './subscription-node-grid';
import { SubscriptionNodeEditor } from './subscription-node-editor';
import { useSubscriptionSources } from './use-subscription-sources';

export function SubscriptionSourcePanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t, i18n } = useTranslation();
  const addSourceRef = useRef<HTMLButtonElement>(null);
  const {
    sources,
    nodes,
    selected,
    setSelected,
    tab,
    setTab,
    search,
    setSearch,
    size,
    setSize,
    setPage,
    error,
    busy,
    editor,
    setEditor,
    form,
    setForm,
    creating,
    setCreating,
    formError,
    setFormError,
    confirmNavigation,
    reload,
    openSource,
    openSettings,
    refresh,
    saveSource,
    toggle,
    displayedSources,
    inManualCollection,
    current,
    pages,
    newSource,
  } = useSubscriptionSources();
  const fields = form && (
    <div className='subscription-settings-fields'>
      <label htmlFor='source-name'>{t('subscriptions.common.name')}</label>
      <input
        disabled={busy}
        id='source-name'
        onChange={(event) => setForm({ ...form, name: event.target.value })}
        value={form.name}
      />
      {(!form.source || form.source.source_kind === 'remote') && (
        <>
          <label htmlFor='source-url'>{t('subscriptions.sources.url')}</label>
          <input
            autoComplete='off'
            disabled={busy}
            id='source-url'
            onChange={(event) => setForm({ ...form, url: event.target.value })}
            placeholder='https://'
            type='url'
            value={form.url}
          />
          <label htmlFor='source-format'>{t('subscriptions.channel.field.format')}</label>
          <SelectField
            disabled={busy}
            id='source-format'
            onValueChange={(value) => setForm({ ...form, format: value })}
            value={form.format}
            items={[
              { value: 'auto', label: t('subscriptions.sources.auto') },
              { value: 'sing-box-json', label: 'sing-box JSON' },
              { value: 'mihomo-yaml', label: 'Mihomo YAML' },
              { value: 'uri-list', label: 'URI' },
            ]}
          />
          <label htmlFor='source-interval'>{t('subscriptions.sources.interval')}</label>
          <SelectField
            disabled={busy}
            id='source-interval'
            onValueChange={(value) => setForm({ ...form, interval: value })}
            value={form.interval}
            items={[...new Set(['0', '60', '360', '720', '1440', form.interval])].map((value) => ({
              value,
              label: value === '0' ? t('subscriptions.sources.onDemand') : `${value} min`,
            }))}
          />
        </>
      )}
    </div>
  );

  return (
    <Tabs
      className='subscription-source-workspace'
      value={tab}
      onValueChange={(value) => value === 'settings' ? void openSettings() : confirmNavigation(() => setTab('nodes'))}
    >
      {error != null && <ErrorNotice error={error} title={t('subscriptions.source.loadFailed')} />}
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='subscription-source-toolbar workspace-toolbar-content'>
          {selected && selected !== 'manual' && (
            <div className='subscription-detail-tabs'>
              <TabsList className='subscriptions-tabs' aria-label={t('subscriptions.sources.settings')}>
                <TabsTrigger value='nodes' disabled={busy}>
                  {t('subscriptions.sources.nodes')}
                </TabsTrigger>
                <TabsTrigger value='settings' disabled={busy}>
                  {t('subscriptions.sources.settings')}
                </TabsTrigger>
              </TabsList>
            </div>
          )}
          <div className='subscription-toolbar-actions'>
            {selected && (
              <Button
                aria-label={t('subscriptions.sources.back')}
                title={t('subscriptions.sources.back')}
                disabled={busy}
                onClick={() => confirmNavigation(() => {
                  setSelected(null);
                  setSearch('');
                  setForm(null);
                  setFormError('');
                  setTab('nodes');
                })}
                size='icon-sm'
                variant='ghost'
              >
                <ArrowLeft aria-hidden='true' />
              </Button>
            )}
            {(tab === 'nodes' || !selected) && (
              <>
                <Button
                  aria-label={t('subscriptions.sources.refresh')}
                  title={t('subscriptions.sources.refresh')}
                  disabled={busy}
                  onClick={() =>
                    selected === 'manual'
                      ? reload()
                      : void refresh(
                        selected
                          ? [selected]
                          : sources
                              .filter((source) => source.enabled && source.source_kind === 'remote')
                              .map((source) => source.id),
                      )
                  }
                  size='icon-sm'
                  variant='ghost'
                >
                  <RefreshCw aria-hidden='true' className={busy ? 'animate-spin' : ''} />
                </Button>
                {(!selected || selected === 'manual') && (
                  <Button
                    ref={addSourceRef}
                    aria-label={t(
                      selected ? 'subscriptions.nodes.add' : 'subscriptions.source.attach',
                    )}
                    title={t(selected ? 'subscriptions.nodes.add' : 'subscriptions.source.attach')}
                    disabled={busy}
                    onClick={() =>
                      selected
                        ? setEditor({ node: null })
                        : (setForm(newSource()), setCreating(true), setFormError(''))
                    }
                    size='icon-sm'
                    variant='ghost'
                  >
                    <CirclePlus aria-hidden='true' />
                  </Button>
                )}
              </>
            )}
          </div>
          {(tab === 'nodes' || !selected) && (
            <div className='subscription-search'>
              <Search aria-hidden='true' />
              <input
                aria-label={t('subscriptions.sources.search')}
                onChange={(event) => {
                  setSearch(event.target.value);
                  setPage(1);
                }}
                placeholder={t('subscriptions.sources.search')}
                value={search}
              />
            </div>
          )}
        </div>
      </ToolbarActions>
      {selected
        ? (
            tab === 'settings' && form
              ? (
                  <TabsContent value='settings' className='subscription-source-settings'>
                    {formError && (
                      <ErrorNotice error={formError} />
                    )}
                    {fields}
                    <footer>
                      <Button disabled={busy} onClick={() => void saveSource()} variant='default'>
                        {t('subscriptions.sources.save')}
                      </Button>
                    </footer>
                  </TabsContent>
                )
              : (
                  <TabsContent value='nodes' className='subscription-node-list'>
                    <SubscriptionNodeGrid
                      key={selected}
                      sourceID={selected}
                      busy={busy}
                      nodes={nodes.filter((node) =>
                        selected === 'manual' ? inManualCollection(node) : node.source_id === selected,
                      )}
                      onOpen={(node) => setEditor({ node })}
                      onVisibility={(node) => void toggle(node)}
                      search={search}
                    />
                  </TabsContent>
                )
          )
        : (
            <>
              <div className='subscription-source-table-scroll'>
                <table className='workspace-table subscription-source-table'>
                  <thead>
                    <tr>
                      <th>{t('subscriptions.tabs.sources')}</th>
                      <th>{t('subscriptions.sources.nodes')}</th>
                      <th>{t('subscriptions.sources.updated')}</th>
                      <th>{t('subscriptions.common.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {displayedSources.slice((current - 1) * size, current * size).map((source) => (
                      <tr key={source.id}>
                        <td>
                          <span className='block truncate' title={source.name}>
                            {source.name}
                          </span>
                        </td>
                        <td>
                          {
                            nodes.filter((node) =>
                              source.id === 'manual'
                                ? inManualCollection(node)
                                : node.source_id === source.id,
                            ).length
                          }
                        </td>
                        <td>
                          {source.updated_at
                            ? new Intl.DateTimeFormat(i18n.language, {
                                dateStyle: 'short',
                                timeStyle: 'short',
                              }).format(new Date(source.updated_at))
                            : '—'}
                        </td>
                        <td>
                          <Button size='sm' disabled={busy} onClick={() => openSource(source.id)} variant='outline'>
                            {t('subscriptions.sources.edit')}
                          </Button>
                          <Button
                            size='sm'
                            disabled={
                              busy || (source.id !== 'manual' && source.source_kind !== 'remote')
                            }
                            onClick={() =>
                              source.id === 'manual' ? reload() : void refresh([source.id])
                            }
                            variant='outline'
                          >
                            {t('subscriptions.sources.refresh')}
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <ListPagination
                page={current}
                pages={pages}
                pageSize={size}
                disabled={displayedSources.length === 0}
                onPageChange={setPage}
                onPageSizeChange={value => {
                  setSize(value);
                  setPage(1);
                }}
              />
            </>
          )}
      {editor && (
        <SubscriptionNodeEditor
          candidates={nodes}
          node={editor.node}
          onClose={() => setEditor(null)}
          onSaved={reload}
        />
      )}
      <Dialog
        onOpenChange={(open) => {
          if (!open && !busy) {
            setCreating(false);
          }
        }}
        onOpenChangeComplete={(open) => {
          if (!open) setForm(current => current?.source ? current : null);
        }}
        open={creating}
      >
        <DialogContent className='subscription-source-dialog' finalFocus={addSourceRef}>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.source.attach')}</DialogTitle>
            <DialogDescription className='sr-only'>
              {t('subscriptions.sources.url')}
            </DialogDescription>
          </DialogHeader>
          {formError && (
            <ErrorNotice error={formError} />
          )}
          {fields}
          <DialogFooter>
            <Button
              disabled={busy}
              onClick={() => {
                setCreating(false);
              }}
              variant='outline'
            >
              {t('common.cancel')}
            </Button>
            <Button disabled={busy} onClick={() => void saveSource()} variant='default'>
              {t('subscriptions.sources.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Tabs>
  );
}
