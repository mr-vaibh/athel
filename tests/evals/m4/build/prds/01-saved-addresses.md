# Saved delivery addresses

## Goal
Customers can save delivery addresses on their profile and list them, so checkout can offer them.

## Requirements
- A customer can add an address to their profile with `POST /v1/users/{id}/addresses`. An address has
  `line1`, `city` and `postcode`; `line2` is optional.
- A customer can list their saved addresses with `GET /v1/users/{id}/addresses`, newest first.
- An address missing `line1`, `city` or `postcode` is rejected with 400 and says which field is missing.
- A customer can save at most 10 addresses; the 11th is rejected with 422.
- Addresses are kept in memory for now; persistence comes later.

## Out of scope
- Editing or deleting addresses.
- Address validation against a postal service.
