## Reach the S3 API and use it as a backup target

The master UI (port 9333) is the `seaweedfs` service. The S3 API is its own `s3` service on port 8333, so give it a separate domain (for example `s3.example.com`) in the app's domain settings and it gets its own HTTPS route.

To use it as a backup target, add a storage target with:

- Endpoint: `https://s3.example.com` (or the private address `http://<host>:8333` on the same host; this needs `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true`).
- Access key and secret key: the generated `S3_ACCESS_KEY` and `S3_SECRET_KEY` of the `s3` service (read them from the app's secrets).
- Path-style addressing on, region any value (for example `us-east-1`).
- Create the bucket first, for example with `aws --endpoint-url https://s3.example.com s3 mb s3://backups`.

See [Backups and storage](/backups-and-storage) for adding the target.
