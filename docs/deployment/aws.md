---
title: "AWS"
weight: 20
description: "How to run Netsy on AWS"
---

# Deploying Netsy on AWS

### Example AWS IAM Policy

On AWS, EC2 instances must be able to use the STS Assume Role permission to assume the role with the example role policy below:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "NetsyS3ObjectOperations",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListObjectsV2",
        "s3:HeadObject",
        "s3:GetObjectAttributes",
        "s3:CreateMultipartUpload",
        "s3:UploadPart",
        "s3:CompleteMultipartUpload",
        "s3:AbortMultipartUpload",
        "s3:ListMultipartUploads",
        "s3:ListParts"
      ],
      "Resource": ["arn:aws:s3:::your-netsy-bucket/*"]
    },
    {
      "Sid": "NetsyS3BucketOperations",
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": ["arn:aws:s3:::your-netsy-bucket"]
    },
    {
      "Sid": "NetsyKMSAccess",
      "Effect": "Allow",
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": "arn:aws:kms:your-region:your-account:key/your-kms-key-id",
      "Condition": {
        "StringEquals": {
          "kms:ViaService": "s3.your-region.amazonaws.com"
        }
      }
    }
  ]
}
```
