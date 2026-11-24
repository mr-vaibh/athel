# Customer profile v2

## Goal
Orders and the public gateway read customer profiles from a versioned endpoint, so the users
service can evolve the profile without breaking them.

## Requirements
- The users service serves a customer's profile at `GET /v2/users/{id}`, with the same fields as
  `GET /v1/users/{id}` today.
- Orders reads an order's customer from the v2 endpoint.
- The gateway exposes the v2 profile to outside callers at `/profile/v2/users/{id}`.
- Nothing breaks while the change rolls out: each service may be deployed on its own, in the
  safe order.
- `GET /v1/users/{id}` is removed once nothing calls it any more, in a later change.

## Out of scope
- New profile fields.
- Payments and notifications.
