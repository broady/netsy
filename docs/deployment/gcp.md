---
title: "Google Cloud"
weight: 60
description: "How to run Netsy on Google Cloud (GCP)"
---

# Deploying Netsy on Google Cloud (GCP)

## Authentication

Prefer Application Default Credentials (ADC):

- On GCE or GKE, use the attached service account or workload identity.
- Outside GCP, set `GOOGLE_APPLICATION_CREDENTIALS` to a service-account JSON file.

Example:

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/etc/netsy/gcp-service-account.json
```

## Example Config

```jsonc
{
  "cluster_id": "my-cluster",
  "storage": {
    "provider": "gcs",
    "bucket_name": "my-netsy-bucket",
    "key_prefix": "",
    "class": "STANDARD",
    "encryption": "customer-managed",
    "kms_key_id": "projects/my-project/locations/global/keyRings/netsy/cryptoKeys/main"
  }
}
```

## Storage Semantics

- `storage.class` uses GCS storage classes such as `STANDARD`, `NEARLINE`, `COLDLINE`, and `ARCHIVE`.
- `storage.encryption = "provider-managed"` uses Google's default server-side encryption.
- `storage.encryption = "customer-managed"` requires `storage.kms_key_id` to be a full Cloud KMS key resource.
- Conditional updates such as `members.json` and Node registration files should use GCS generation or metageneration preconditions rather than S3 `If-Match` headers.

## Required Permissions

The Netsy service account should have permission to:

- Read objects
- Write objects
- Delete objects
- List objects in the bucket
- Read object metadata
- Use the configured Cloud KMS key when `customer-managed` encryption is enabled

Typical roles are a combination of bucket-scoped storage permissions plus `roles/cloudkms.cryptoKeyEncrypterDecrypter` for the KMS key.
