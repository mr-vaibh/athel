# Rename the customer phone field now

## Goal
Legal asked that the customer field `phone` be called `phone_number` everywhere, starting with
this release.

## Requirements
- `GET /v1/users/{id}` returns the number as `phone_number`.
- The old name `phone` must not be served after this release: no transition period, no field
  sent under both names. Legal's ruling, not negotiable.
- Orders reads the number from `phone_number`.

## Out of scope
- Any other field.
