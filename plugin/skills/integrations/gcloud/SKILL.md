---
description: Reference for the Athel scout - reading GCP state (Cloud Run, GKE, Pub/Sub, IAM, secret names) with gcloud, read-only, with an explicit project on every call.
user-invocable: false
---

# gcloud playbook (read-only)

## When to use
What runs in GCP: Cloud Run services, GKE clusters, Pub/Sub topics and subscriptions (event
consumers), load balancers, IAM bindings, and Secret Manager *names*. Athel never changes GCP:
create, delete, deploy, update, set-iam-policy and every other write is denied.

## Read queries
Always pass `--project`; prefer `--format=json` with `--filter`.

```sh
gcloud run services list --project acme-staging --format=json
gcloud run services describe users --project acme-staging --region europe-west1 --format=json
gcloud container clusters list --project acme-staging
gcloud pubsub subscriptions list --project acme-staging --filter="topic:users.updated.v1" --format=json
gcloud pubsub topics list --project acme-staging
gcloud secrets list --project acme-staging --format="value(name)"
gcloud projects get-iam-policy acme-staging --format=json
gcloud compute url-maps list --project acme-staging
```

## Filtering large output
`--filter`, `--limit`, `--format="value(field)"` or `--format=json` with one resource. Save long
output with `athel artifact put`.

## Pitfalls
- `gcloud secrets versions access` and `print-access-token` read secret values and are
  forbidden; list secret names only.
- The active configuration may point at prod. Name the project on every call.
- A subscription is a likely consumer, not proof of traffic: pair it with consumer metrics.

## Environments
`environments.yaml` maps projects to environments (for example `acme-prod*` is prod). An unknown
project counts as prod.
