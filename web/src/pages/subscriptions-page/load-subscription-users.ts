import type { ApiClient, SubscriptionCursor, SubscriptionUser } from '@/api/api-client';

export async function loadSubscriptionUsers(
  client: Pick<ApiClient, 'listSubscriptionUsers'>,
  signal?: AbortSignal,
): Promise<SubscriptionUser[]> {
  const users: SubscriptionUser[] = [];
  let cursor: SubscriptionCursor | undefined;

  do {
    const page = await client.listSubscriptionUsers({
      beforeID: cursor?.id,
      beforeTime: cursor?.created_at,
      limit: 100,
    }, signal);
    users.push(...page.items);
    cursor = page.next;
  } while (cursor !== undefined);

  return users;
}
