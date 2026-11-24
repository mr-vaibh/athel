# Phone numbers in E.164

## Goal
Orders sends notifications to phone numbers it can always dial: every phone number Athel's shop
hands out is in E.164 form (`+` and digits).

## Requirements
- The users service stores, next to the phone number as the customer typed it, the number in
  E.164 form, filled for existing customers too.
- `GET /v1/users/{id}` also returns `phone_e164`; every field it returns today keeps its meaning.
- Orders uses `phone_e164` when it reads a customer.
- A number that cannot be normalised leaves `phone_e164` empty; it is never guessed.

## Out of scope
- Changing how customers enter their number.
- Removing the original `phone` field.
