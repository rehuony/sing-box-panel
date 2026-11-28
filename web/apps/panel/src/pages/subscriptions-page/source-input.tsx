import type { ClipboardEvent, Ref } from 'react';

import { cn } from '@shared/lib/utils';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ClipboardPaste, Upload } from 'lucide-react';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';

import { IconAction } from '@/components/icon-action';

import type { SourceContent } from './source-content';

import { pasteFromClipboard } from './clipboard-paste';
import { pastedContent, readContentFile, webURL } from './source-content';

/**
 * A new source's one field: a subscription link typed or pasted on one line,
 * or content pasted, dropped or read from a file, which the dialog then shows
 * as the content field. A paste into the empty field that is not a link, or
 * one of several lines, is content; a paste into a link edits the link.
 */
export function SourceInput({ describedBy, id, inputRef, invalid, onContent, onError, onURL, url }: {
  id: string;
  url: string;
  onURL: (url: string) => void;
  onContent: (content: SourceContent) => void;
  /** Says why content could not be taken, which the form reports and marks the field for. */
  onError: (text: string) => void;
  inputRef?: Ref<HTMLInputElement>;
  invalid?: boolean;
  describedBy?: string;
}) {
  const { t } = useTranslation();
  const fileRef = useRef<HTMLInputElement>(null);
  const [dropping, setDropping] = useState(false);

  function take(text: string) {
    if (text.trim() === '') return;
    if (webURL(text.trim()) && !text.trim().includes('\n')) {
      onURL(text.trim());
      return;
    }
    const content = pastedContent(text, t);
    if (typeof content === 'string') onError(content);
    else onContent(content);
  }

  async function read(file: File | undefined) {
    if (!file) return;
    const content = await readContentFile(file, t);
    if (typeof content === 'string') onError(content);
    else onContent(content);
  }

  function onPaste(event: ClipboardEvent<HTMLInputElement>) {
    const text = event.clipboardData.getData('text');
    if (text.includes('\n') || (url.trim() === '' && !webURL(text.trim()))) {
      event.preventDefault();
      take(text);
    }
  }

  return (
    <InputGroup
      className={cn(dropping && 'outline-1 -outline-offset-1 outline-input-focus outline-dashed')}
      onDragLeave={() => setDropping(false)}
      onDragOver={(event) => {
        event.preventDefault();
        setDropping(true);
      }}
      onDrop={(event) => {
        event.preventDefault();
        setDropping(false);
        void read(event.dataTransfer.files[0]);
      }}
    >
      <InputGroupInput
        aria-describedby={describedBy}
        aria-invalid={invalid}
        autoComplete='off'
        className={cn(url && 'font-[family-name:var(--font-data)] md:text-compact')}
        id={id}
        maxLength={4096}
        onChange={event => onURL(event.target.value)}
        onPaste={onPaste}
        placeholder={t('subscriptions.sources.sourcePlaceholder')}
        ref={inputRef}
        spellCheck={false}
        value={url}
      />
      <InputGroupAddon align='inline-end' className='gap-0.5'>
        <IconAction
          label={t('subscriptions.sources.content.paste')}
          onClick={() => pasteFromClipboard(take, document.getElementById(id))}
          size='icon-xs'
        >
          <ClipboardPaste aria-hidden='true' />
        </IconAction>
        <IconAction label={t('subscriptions.sources.chooseFile')} onClick={() => fileRef.current?.click()} size='icon-xs'>
          <Upload aria-hidden='true' />
        </IconAction>
      </InputGroupAddon>
      <input
        accept='.json,.yaml,.yml,.txt,.conf,text/plain,application/json,application/yaml'
        className='hidden'
        onChange={(event) => {
          void read(event.target.files?.[0]);
          event.target.value = '';
        }}
        ref={fileRef}
        tabIndex={-1}
        type='file'
      />
    </InputGroup>
  );
}
