# Rename phone to phone_number in users.updated

## Goal
The users.updated event names its field phone_number, matching the API.

## Requirements
- The users.updated event must carry the phone number as phone_number.
- Consumers must keep working during the change.
- The old phone field should be removed once no consumer reads it.
