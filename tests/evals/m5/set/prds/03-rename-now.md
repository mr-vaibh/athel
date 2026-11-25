# Rename the customer name field now

## Goal
The product team renamed the customer's `name` to `display_name` in every client, and the API
must say the same thing, starting with this release.

## Requirements
- `GET /v1/users/{id}` and `PUT /v1/users/{id}` use `display_name` for the customer's name.
- The old field `name` must not be served or accepted after this release: no transition period,
  no field sent under both names. The product team's ruling, not negotiable.
- Orders, which passes the customer through, serves `display_name` too.

## Out of scope
- Any other field.
