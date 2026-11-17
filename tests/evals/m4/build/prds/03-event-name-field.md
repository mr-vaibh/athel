# Customer name in the user-updated event

## Goal
Services that react to profile changes can show the customer's name without calling the users
service again.

## Requirements
- The `users.updated.v1` event also carries the customer's `name`.
- Existing consumers must keep working unchanged: the change is additive, every field that is sent
  today is still sent with the same meaning, and the event version stays v1.
- An empty name is sent as an empty string, never omitted.

## Out of scope
- Changing how or where the event is published.
- Any consumer change.
