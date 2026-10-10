# Tamga Deploy

Production-ready deployment için üç artefakt vardır.

## Helm

```bash
helm upgrade --install tamga-proxy ./helm/tamga-proxy \
  --namespace tamga --create-namespace \
  --set image.tag=0.5.0 \
  --set env.TAMGA_ADMIN_KEY=$(openssl rand -hex 32)
```

Helm chart şu bileşenleri kurar:

- `Deployment` — read-only root filesystem, drop-ALL kabiliyet seti, non-root user
- `Service` (ClusterIP)
- `ConfigMap` — `policy.yaml` inline
- `ServiceAccount`
- `PodDisruptionBudget`
- `HorizontalPodAutoscaler` (opsiyonel)

## Terraform — AWS EKS (Frankfurt)

```bash
cd terraform/aws
terraform init
terraform apply \
  -var="tamga_admin_key=$(openssl rand -hex 32)" \
  -var="region=eu-central-1"
```

`eu-central-1` Türkiye için en düşük gecikmeli AWS bölgesidir. KVKK uyumu için
veri ikametgahını `tr` olarak işaretleyen policy kullanılır.

## SQL Migrations

Migrations follow the `golang-migrate` format (`NNN_name.up.sql` + `NNN_name.down.sql`).

```bash
# Every migration, in order. The proxy reads the same address from TAMGA_DB_URL.
for f in migrations/*.up.sql; do psql "$TAMGA_DB_URL" -v ON_ERROR_STOP=1 -f "$f"; done
```

They are written to be run again without harm. To roll one back, run its
`.down.sql`:

```bash
psql "$TAMGA_DB_URL" -f migrations/018_drop_tenant_rls.down.sql
```
