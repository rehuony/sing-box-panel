/**
 * A message as a one-line slot shows it: without the punctuation that closes
 * a sentence, whether the text came from the panel or from the server. A
 * question mark and an ellipsis stay, since they say something.
 */
export function messageLine(text: string): string {
  return text.replace(/[\s.。．!！;；]+$/u, '');
}
