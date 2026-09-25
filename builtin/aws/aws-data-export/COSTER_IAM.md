# IAM for AWS cost reporting

Nullstone reads AWS cost data with the same role or user that is connected as the provider.
There are two tiers; the second is optional and adds to the first.

## Tier 1: Cost Explorer (required)

Used for every month and, when a Data Export is configured, as the independent total that
each ingested month is reconciled against.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

## Tier 2: Data Export (optional, FOCUS 1.2)

Adds ListCost, ContractedCost and resource-level detail. The export is created by the customer
in the payer account and delivered to an S3 bucket they own; Nullstone only reads it.

Export definition (`aws bcm-data-exports create-export`):

| Setting | Value |
|---|---|
| Table | `FOCUS_1_2_AWS` |
| Table configuration | `TIME_GRANULARITY = DAILY` |
| Format / compression | `TEXT_OR_CSV` / `GZIP` |
| Output type | `CUSTOM` |
| Overwrite | `OVERWRITE_REPORT` |
| Refresh cadence | `SYNCHRONOUS` |

Read-only policy on the connected Nullstone role, scoped to the export prefix. Replace
`<bucket>`, `<prefix>` and `<export-name>`; the Nullstone path is
`s3://<bucket>/<prefix>/<export-name>`.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "s3:GetBucketLocation",
      "Resource": "arn:aws:s3:::<bucket>"
    },
    {
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::<bucket>",
      "Condition": { "StringLike": { "s3:prefix": "<prefix>/<export-name>/*" } }
    },
    {
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::<bucket>/<prefix>/<export-name>/*"
    }
  ]
}
```

Bucket policy that lets Data Exports deliver (payer account `<payer>`):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": { "Service": "bcm-data-exports.amazonaws.com" },
      "Action": ["s3:PutObject", "s3:GetBucketPolicy"],
      "Resource": ["arn:aws:s3:::<bucket>", "arn:aws:s3:::<bucket>/*"],
      "Condition": {
        "StringLike": {
          "aws:SourceAccount": "<payer>",
          "aws:SourceArn": "arn:aws:bcm-data-exports:us-east-1:<payer>:export/*"
        }
      }
    }
  ]
}
```
