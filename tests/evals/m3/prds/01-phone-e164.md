# Phone numbers in E.164

## Goal
Customers can update their phone number, and every phone number is stored in E.164.

## Requirements
- A signed-in customer can change their own phone number.
- The users service must accept local formats and store the number in E.164.
- Every service that shows or uses the phone number must see the new number within 1 second.
- Numbers that cannot be converted must be rejected with a clear error.

## Out of scope
- Changing how notifications sends SMS.
