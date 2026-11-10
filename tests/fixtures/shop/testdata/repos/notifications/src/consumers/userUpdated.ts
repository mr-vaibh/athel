// Keeps a local copy of each customer's phone for SMS. users owns the number; this is a cache.
export const TOPIC = "users.updated.v1";
export const GROUP = "notifications";

export function onUserUpdated(event: { id: string; phone?: string }): string {
  return `cache phone for ${event.id}`;
}
