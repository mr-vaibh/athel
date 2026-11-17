# Clear errors from the users client in orders

## Goal
When orders cannot read a customer from the users service, it gets a clear, typed error instead of
a raw HTTP response it has to inspect itself.

## Requirements
- `Users.Get` in the orders service returns the decoded customer (`id`, `name`, `phone`) instead of
  the raw HTTP response.
- When the users service answers 404, `Users.Get` returns an error that callers can match as
  "customer not found".
- When the users service answers any other non-2xx status, or the call fails, `Users.Get` returns
  an error that names the status or the cause and can be matched as "users unavailable".
- Calls to the users service time out after 2 seconds and use the caller's context.

## Out of scope
- Retries.
- Any other client in orders.
