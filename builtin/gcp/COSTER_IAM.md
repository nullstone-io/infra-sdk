# GCP Coster - Required IAM Permissions

The GCP Coster queries billing data from a BigQuery billing export table. The service account or identity used must have the following permissions.

## BigQuery Permissions

| Permission | Purpose |
|------------|---------|
| `bigquery.jobs.create` | Run queries in the project specified by `GcpProjectId()` |
| `bigquery.tables.getData` | Read rows from the billing export table |
| `bigquery.tables.get` | Access table metadata |

## Predefined Roles

The simplest way to grant these permissions is with the following predefined roles:

- **`roles/bigquery.jobUser`** on the project where queries run (`GcpProjectId()`)
- **`roles/bigquery.dataViewer`** on the dataset containing the billing export table (`BillingDataset()`)

## Example gcloud Setup

```sh
# Allow the service account to run BigQuery queries
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:SA_EMAIL" \
  --role="roles/bigquery.jobUser"

# Allow the service account to read the billing export dataset
bq update --dataset \
  --source=<(echo '[{"role":"READER","specialGroup":"projectReaders"}]') \
  PROJECT_ID:BILLING_DATASET

# Or grant dataViewer at the dataset level
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:SA_EMAIL" \
  --role="roles/bigquery.dataViewer" \
  --condition="expression=resource.name.startsWith('projects/PROJECT_ID/datasets/BILLING_DATASET'),title=billing-export-only"
```

## Notes

- The billing export table is created by GCP when you enable billing export to BigQuery. It is not managed by this SDK.
- If the billing export dataset is in a different project than `GcpProjectId()`, the service account needs `bigquery.tables.getData` in that project as well.
- No write permissions are needed. All operations are read-only queries.
