# Investigation · ASK-001 · data_owner

**Question:** Which service owns the customer phone number?

The users service (github.com/acme/users) owns the customer phone number. Confidence: likely (code only; no runtime source reached). Evidence: users/migrations/001_users.sql defines users.phone; users/api/user.go PUT /v1/users/{id} writes it and publishes a user-updated event, commented 'Phone is owned here: users writes it, everyone else reads it'; users/main.go says the same. orders/clients/users.go only reads it via GET /v1/users/{id}. No other repo (gateway, orders, payments) writes a phone column. Runtime: kubectl on gke_acme-prod_eu found no deploy labelled app=users (label may differ; staging not checked), so no runtime confirmation. Drift (unverified, code only): orders still calls the removed /v1/users/{id}/legacy-profile endpoint from the receipt job. Gaps: Grafana/Tempo traces, live DB schema, GitHub PRs not queried. Active work by others: not checked. Suspicious content: none.

## Claim block (athel/investigation@1, checked by the athel CLI)

```yaml
schema: athel/investigation@1
ask: ASK-001
question: Which service owns the customer phone number?
type: data_owner
scope:
    services: []
    repos:
        - github.com/acme/users
        - github.com/acme/orders
        - github.com/acme/gateway
        - github.com/acme/payments
complete: false
budget:
    max_tool_calls: 60
    max_minutes: 10
    used_tool_calls: 26
    used_minutes: 6
claims:
    - id: C1
      text: users service owns the customer phone number (users.users.phone, written via PUT /v1/users/{id})
      subject:
        kind: service
        name: users
      predicate: owns_data
      object:
        kind: table
        name: users.users.phone
      env: any
      confidence: likely
      evidence:
        - kind: code
          ref: users/migrations/001_users.sql:4
        - kind: code
          ref: users/api/user.go:11,35
      facts: []
    - id: C2
      text: orders only reads the phone from users via GET /v1/users/{id}
      subject:
        kind: service
        name: orders
      predicate: reads_data
      object:
        kind: table
        name: users.users.phone
      env: any
      confidence: likely
      evidence:
        - kind: code
          ref: orders/clients/users.go:9-12
      facts: []
drift:
    - text: orders LegacyProfile still calls /v1/users/{id}/legacy-profile, which users removed in 2026-09 (code only, not runtime-verified)
      evidence:
        - kind: code
          ref: orders/clients/users.go:14-16
        - kind: code
          ref: users/api/user.go:1
gaps:
    - source: kubectl
      code: E-TOOL-SOURCE-MISSING
      reason: no deploy labelled app=users in gke_acme-prod_eu; staging not checked
      login: gcloud container clusters get-credentials <cluster> --project acme-staging
      effect: runtime claims stay unverified, ownership at most likely
    - source: grafana
      code: E-TOOL-SOURCE-MISSING
      reason: traces and live DB schema not queried
      login: gcloud auth login
      effect: ownership not cross-checked against runtime
    - source: schema or migrations
      code: E-TOOL-SOURCE-MISSING
      reason: no claim answering the question used schema or migrations
      login: null
      effect: claims that needed it are at most likely
active_work: []
health: []
suspicious: []
facts_used: []
facts_created: []
```
