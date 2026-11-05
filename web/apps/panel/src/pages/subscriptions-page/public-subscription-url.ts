export function buildPublicSubscriptionURL(
  plaintextToken: string,
  channelID: string,
  baseURI = document.baseURI,
): string {
  const relativePath = `sub/${encodeURIComponent(plaintextToken)}/${encodeURIComponent(channelID)}`;
  return new URL(relativePath, baseURI).href;
}
