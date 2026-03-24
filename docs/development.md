---
title: "Development"
weight: 40
description: "How to work on/develop Netsy locally"
---

# Developing Netsy

Start a localstack s3 server:

```
docker compose up -d
```

Generate some certificates:

```
./scripts/certs.sh
```

Start a netsy dev server:

```
./dev.sh
```

(or with database reset first):

```
rm -f temp/data/db.sqlite3*; ./dev.sh
```

If you need to reset S3 bucket contents:

```
AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test aws --endpoint-url="http://localhost:4566" s3 rm s3://netsy-dev --recursive
```

And if you want to test with a kube-apiserver container:

```
./scripts/kube-apiserver.sh
```

You can also run `etcdctl` with a helper script (which wires up the correct certs and endpoint):

```
./scripts/etcdctl.sh
```
