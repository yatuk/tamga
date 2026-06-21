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
psql "$TAMGA_POSTGRES_URL" -f migrations/001_init.up.sql
psql "$TAMGA_POSTGRES_URL" -f migrations/002_indexes.up.sql
psql "$TAMGA_POSTGRES_URL" -f migrations/003_output_scan.up.sql
psql "$TAMGA_POSTGRES_URL" -f migrations/004_audit_policy.up.sql
psql "$TAMGA_POSTGRES_URL" -f migrations/005_retention_run_log.up.sql
psql "$TAMGA_POSTGRES_URL" -f migrations/006_model_metadata.up.sql
```

To roll back the most recent migration:

```bash
psql "$TAMGA_POSTGRES_URL" -f migrations/006_model_metadata.down.sql
```
