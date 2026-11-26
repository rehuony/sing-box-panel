import type { TFunction } from 'i18next';

/** The largest content a source holds. */
export const MAX_CONTENT_BYTES = 4 * 1024 * 1024;
/** Content given in the dialog: pasted, or read from a file. */
export interface SourceContent {
  size: number;
  text: string;
  origin: { kind: 'paste'; lines: number } | { kind: 'file'; name: string };
}

function pasted(text: string): SourceContent {
  return {
    origin: { kind: 'paste', lines: text.split(/\r?\n/).filter(line => line.trim() !== '').length },
    size: new Blob([text]).size,
    text,
  };
}

/** Pasted text as a source's content, or why it cannot be one. */
export function pastedContent(text: string, t: TFunction): SourceContent | string {
  const content = pasted(text);
  return content.size > MAX_CONTENT_BYTES ? t('subscriptions.sources.fileTooLarge') : content;
}

/** A file read as a source's content, or why it cannot be one. */
export async function readContentFile(file: File, t: TFunction): Promise<SourceContent | string> {
  if (file.size > MAX_CONTENT_BYTES) return t('subscriptions.sources.fileTooLarge');
  try {
    return { origin: { kind: 'file', name: file.name }, size: file.size, text: await file.text() };
  } catch {
    return t('subscriptions.sources.content.unreadable');
  }
}

/** An http or https URL, the only kind a source fetches. */
export function webURL(value: string): boolean {
  try {
    return ['http:', 'https:'].includes(new URL(value).protocol);
  } catch {
    return false;
  }
}
