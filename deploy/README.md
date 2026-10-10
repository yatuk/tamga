# Deployment

| Path | What it is |
|---|---|
| `docker-compose.yml` | The full stack on one host: proxy, analyzer, scanner service, console, PostgreSQL, Redis, NATS, Jaeger. See the [root README](../README.md#quick-start). |
| `helm/tamga-proxy/` | A Helm chart for the proxy alone. |
| `terraform/aws/` | An EKS cluster on AWS with that chart installed in it. |
| `migrations/` | The PostgreSQL schema. |
| `backup/` | Database backup and restore scripts. See [backup/README.md](backup/README.md). |

No container image is published. For Helm and Terraform, build the proxy
image from `proxy/Dockerfile`, push it to a registry your cluster can pull
from, and pass its repository and tag.

## Helm

The chart installs the proxy only:

- a `Deployment` (non-root user 1001, read-only root filesystem, all
  capabilities dropped), listening on port 8443,
- a `Service` (ClusterIP, port 8443); the port is named `http`, or `https`
  when the proxy serves TLS,
- a `Secret` with the credentials, unless you supply your own,
- a `ServiceAccount`,
- a `PodDisruptionBudget`, when it leaves a pod free to evict (see below),
- a `HorizontalPodAutoscaler` when `autoscaling.enabled` is true,
- a `ConfigMap` with the policy when `policy.inline` is set.

It does not install PostgreSQL, Redis, the analyzer, NATS or the console, and
it has no Ingress or NetworkPolicy.

```bash
helm upgrade --install tamga-proxy ./helm/tamga-proxy \
  --namespace tamga --create-namespace \
  --set image.repository=registry.example.com/tamga \
  --set image.tag=<your tag> \
  --set secrets.adminKey="$(openssl rand -hex 32)"
```

The render fails when no admin key is given.

### Credentials

`TAMGA_ADMIN_KEY`, `TAMGA_DB_URL` and `REDIS_URL` reach the pod from a Secret.
Choose one:

- Set `secrets.adminKey`, and `secrets.databaseUrl` and `secrets.redisUrl` if
  you use them. The chart creates the Secret. The values then also sit in the
  Helm release, so prefer the next option for production.
- Create a Secret yourself with the keys `TAMGA_ADMIN_KEY` and, if used,
  `TAMGA_DB_URL` and `REDIS_URL`, and set `secrets.existingSecret` to its
  name. The other `secrets.*` values are then ignored.

  ```bash
  kubectl -n tamga create secret generic tamga-proxy-credentials \
    --from-literal=TAMGA_ADMIN_KEY="$(openssl rand -hex 32)" \
    --from-literal=TAMGA_DB_URL='postgresql://tamga:<password>@postgres:5432/tamga'
  ```

Without `TAMGA_DB_URL` the proxy stores nothing and shows recent requests
from memory. Without `REDIS_URL` each replica keeps its own rate-limit, budget
and cache state.

### Other settings

| Value | Default | Meaning |
|---|---|---|
| `image.repository`, `image.tag` | `ghcr.io/yatuk/tamga`, the chart's `appVersion` | The image to run. Nothing is published at the default; set both. |
| `replicaCount` | `2` | Replicas, when autoscaling is off. |
| `containerPort` | `8443` | The port the proxy listens on, passed as `TAMGA_PROXY_PORT`. |
| `service.type`, `service.port` | `ClusterIP`, `8443` | The Service. |
| `analyzer.grpcAddr`, `analyzer.httpUrl` | empty | The analyzer's gRPC address (`TAMGA_ANALYZER_ADDR`) and HTTP base URL (`TAMGA_ANALYZER_HTTP_URL`). See [Analyzer](#analyzer). |
| `analyzer.sendContent` | `false` | Whether the proxy sends the analyzer the text of forwarded requests (`TAMGA_ANALYZER_SEND_CONTENT`). See [Analyzer](#analyzer). |
| `podDisruptionBudget.enabled`, `.minAvailable` | `true`, `1` | See [Disruption budget](#disruption-budget). |
| `env` | `{}` | Other proxy environment variables, by the names in [docs/operations.md](../docs/operations.md#configuration). |
| `policy.inline` | empty | A complete policy document. Empty uses the policy built into the image. |
| `extraVolumes`, `extraVolumeMounts` | empty | For example a TLS certificate. |

The chart sets `TAMGA_ADMIN_KEY`, `TAMGA_DB_URL`, `REDIS_URL`,
`TAMGA_PROXY_PORT` and `TAMGA_POLICY_PATH` itself and refuses them under
`env`.

A policy given in `policy.inline` is mounted from a ConfigMap at
`/etc/tamga/policy.yaml`. The proxy does not start on a policy it cannot
parse, so check the document first with `POST /api/v1/policies/validate` on a
running proxy.

The console can show the policy and cannot save changes to it, with or without
`policy.inline`: the ConfigMap mount is read-only, and the image's own copy
sits on the read-only root filesystem. A save is answered with 409
`policy_read_only`. Change the policy by changing `policy.inline` and
upgrading the release; the pods restart because the ConfigMap's checksum is a
pod annotation.

The only writable path in the pod is `/tmp` (an `emptyDir`). Without
`TAMGA_DB_URL` the proxy keeps policy history in files under
`./data/policy-history`, which the read-only root filesystem refuses, so it
logs `policy history disabled` and runs without it. Give it a database, or set
`env.TAMGA_POLICY_HISTORY_DIR` to a directory on a volume you mount with
`extraVolumes`.

The resources are named after the release: `<release>-tamga-proxy`, or just
the release name when it already contains `tamga-proxy`.

### Analyzer

The chart does not deploy the analyzer. Run it yourself (the image is built
from `analyzer/Dockerfile`; it listens on 50051 for gRPC and 8444 for HTTP)
and tell the proxy where it is:

```bash
  --set analyzer.grpcAddr=analyzer.tamga.svc:50051 \
  --set analyzer.httpUrl=http://analyzer.tamga.svc:8444
```

With both left empty the proxy uses its built-in defaults, `localhost:50051`
and `http://localhost:8444`, and nothing listens there inside the pod. What
does not work then:

- The PDF reports, `GET /api/v1/reports/owasp/pdf` and
  `GET /api/v1/reports/incident/pdf`, answer 503: the proxy fetches them from
  the analyzer over HTTP.
- Nothing the proxy hands to the analyzer over gRPC is analysed.

Proxying, the inline scanners and the policy do not depend on the analyzer.
The render fails when an address is given both here and under `env`.

The proxy sends the analyzer no request text unless `analyzer.sendContent` is
`true`, which the chart passes as `TAMGA_ANALYZER_SEND_CONTENT=true`. With it
on, each request the proxy forwarded goes to `analyzer.grpcAddr` as it was
forwarded (masked where the policy masked it; a blocked request is never
sent) for a second look after the decision; the findings are logged as a
count and change no verdict. Leave it off unless the analyzer runs where that
text may go: with the analyzer's LLM judge on (`TAMGA_LLM_JUDGE`, set on the
analyzer), the analyzer sends the start of the text on to that provider. The
PDF reports work either way. A release that already sets the variable under
`env` keeps working; the render fails when it is set in both places.

```bash
  --set analyzer.sendContent=true
```

### Disruption budget

A `PodDisruptionBudget` with `minAvailable: 1` over a single replica allows no
eviction at all, and `kubectl drain` on that pod's node waits until someone
deletes the budget. The chart therefore renders the budget only when it cannot
block: when the lowest replica count (`replicaCount`, or
`autoscaling.minReplicas` when autoscaling is on) is greater than
`podDisruptionBudget.minAvailable`. With the defaults (two replicas,
`minAvailable: 1`) it is rendered. With `replicaCount: 1` it is not, and a
drain takes the only pod down until it is rescheduled. A `minAvailable` given
as a percentage is rendered as it is. `podDisruptionBudget.enabled: false`
leaves it out in every case.

### Probes

Liveness and readiness both call `GET /health`, which needs no key and does
not depend on the database, Redis or the analyzer. They use HTTPS when
`env.TAMGA_TLS_CERT` and `env.TAMGA_TLS_KEY` are both set; the port is then
named `https` on the container and the Service, and `http` otherwise. With
`TAMGA_MTLS_STRICT_VERIFY=true` the kubelet has no client certificate and the
probes fail; the chart has no answer to that.

`GET /api/v1/health/detailed` is not used as a probe: it answers 503 when any
dependency is down, which would take every replica out of the Service at once.

### Changes from chart 0.1.0

Chart 0.1.0 set variables the proxy does not read, so an install ran with no
database, no Redis and the image's built-in policy whatever the values said.

| 0.1.0 | 0.2.0 |
|---|---|
| `env.TAMGA_ADMIN_KEY` (default `change-me`) | `secrets.adminKey` or `secrets.existingSecret`; no default |
| `env.TAMGA_POSTGRES_URL` | `secrets.databaseUrl` (`TAMGA_DB_URL`) |
| `env.TAMGA_REDIS_URL` | `secrets.redisUrl` (`REDIS_URL`) |
| `env.TAMGA_POLICY_FILE` | set by the chart (`TAMGA_POLICY_PATH`) when `policy.inline` is given |
| `env.TAMGA_MTLS_CLIENT_CA` | `env.TAMGA_MTLS_CLIENT_CA_FILE` |
| `env.TAMGA_LOG_LEVEL` | removed; the proxy reads no such variable |
| container and Service port 8080 | 8443 |
| `policy.inline` default in a format the proxy rejects | empty |
| `podSecurityContext.runAsUser: 65532` | `1001`, the user in `proxy/Dockerfile` |
| `ingress.*`, `networkPolicy.*` | removed; no template used them |

## Terraform (AWS EKS)

`terraform/aws/main.tf` creates a VPC with three private and three public
subnets and one NAT gateway, an EKS cluster (the Kubernetes version you pass
as `cluster_version`) with a managed node group of two to ten `t3.large` nodes, a `tamga` namespace, and a Helm
release of the chart above.

```bash
cd terraform/aws
terraform init
terraform apply \
  -var="cluster_version=<a version EKS supports>" \
  -var="tamga_admin_key=$(openssl rand -hex 32)" \
  -var="proxy_image_repository=registry.example.com/tamga" \
  -var="proxy_image_tag=<your tag>"
```

| Variable | Default | Meaning |
|---|---|---|
| `cluster_version` | none, required | Kubernetes version of the cluster, as `major.minor`. Take one that is in standard support in the [EKS version list](https://docs.aws.amazon.com/eks/latest/userguide/kubernetes-versions.html). It has no default because EKS retires versions and a default in the file goes stale. |
| `tamga_admin_key` | none, required | The management API key. Passed to the chart as `secrets.adminKey`. |
| `proxy_image_tag` | none, required | Tag of the image you pushed. |
| `proxy_image_repository` | `ghcr.io/yatuk/tamga` | Repository of that image. |
| `region` | `eu-central-1` | AWS region. |
| `cluster_name` | `tamga` | Name of the cluster and prefix of the VPC. |

Outputs: `cluster_endpoint` and `kubeconfig_command`.

The three subnets of each kind go into the first three availability zones
that AWS reports for the region (`aws_availability_zones`, Local Zones left
out); a region with fewer than three fails at plan time.

`.terraform.lock.hcl` pins the providers (aws, kubernetes and helm, and the
four the VPC and EKS modules bring: cloudinit, null, time, tls) to
exact versions with their checksums for Linux (amd64, arm64), macOS (arm64)
and Windows (amd64). `terraform init` uses those; `terraform init -upgrade`
moves them within the constraints in `main.tf` and rewrites the file. The two
registry modules (VPC, EKS) are not covered by the lock file: they follow
their `~>` constraints.

Things to know before applying:

- The admin key is stored in the Terraform state. Protect the state.
- The cluster's API endpoint is public (`cluster_endpoint_public_access`).
- The identity that runs Terraform becomes a cluster administrator
  (`enable_cluster_creator_admin_permissions`); the namespace and the Helm
  release are created as that identity, through `aws eks get-token`, so the
  AWS CLI must be installed where Terraform runs.
- The release is named `tamga-proxy`, and so are the Deployment and the
  Service: `tamga-proxy.tamga.svc:8443`.
- The release sets only the admin key and the image. The proxy therefore runs
  with no database and no Redis; add them and extend the release, or install
  the chart yourself with a values file.
- The region does not make a deployment compliant with any regulation. The
  proxy's own residency check is `TAMGA_REGION` against `data.residency` in
  the policy, and it only logs a mismatch at startup.

## SQL migrations

`migrations/` holds numbered pairs, `NNN_name.up.sql` and `NNN_name.down.sql`.
Nothing in the repository applies them for you outside Docker Compose, and
nothing records which ones a database has had.

- Docker Compose mounts every `.up.sql` into the PostgreSQL image's init
  directory, so a new, empty data volume gets the whole schema. An existing
  volume is not touched.
- On an empty database of your own, run them all in order:

  ```bash
  for f in migrations/*.up.sql; do psql "$TAMGA_DB_URL" -v ON_ERROR_STOP=1 -f "$f"; done
  ```

- On a database that already has the schema, run only the files that are new
  to it. `001`, `010` and `011` fail when run a second time.

To roll one back, run its `.down.sql`:

```bash
psql "$TAMGA_DB_URL" -f migrations/019_drop_unused_tables.down.sql
```

Roll back in reverse order. `019` drops tables that the down files of `002`,
`011` and `018` refer to, so those need `019` rolled back first.

Upgrade order and the notes on `017`, `018` and `019` are in
[docs/operations.md](../docs/operations.md#upgrading-the-database).
