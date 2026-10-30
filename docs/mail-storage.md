# Mail storage

Each mail service brings its own S3 compatible bucket. The connection is
owned by one service and holds backups, mailbox archives and exports. Every
object lives under `<service-id>/`, and mailbox addresses appear only as
sha256 hashes in keys.

## Key layout

- `<service-id>/backups/<rfc3339>.tar.zst.age`
- `<service-id>/archive/<mailbox-hash>/<yyyy-mm>.mbox.zst.age`
- `<service-id>/exports/<id>.zip.age`
- `<service-id>/.probe/` (throwaway connection test objects)

Objects are encrypted on the mail node before upload, so the bucket never
holds plaintext. The secret access key is sealed in the secrets store under
`mail-storage/<service-id>` and is never shown again after saving.

## Connection test

Connecting runs four steps and reports each one: put, get, list, delete.
A key that can write but not delete is called out as such, because
retention and uninstall cleanup need delete.

## Least privilege on AWS S3

Replace `BUCKET` and `SERVICE_ID`.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::BUCKET/SERVICE_ID/*"
    },
    {
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::BUCKET",
      "Condition": { "StringLike": { "s3:prefix": ["SERVICE_ID/*"] } }
    }
  ]
}
```

## Cloudflare R2

R2 tokens scope to a bucket, not a prefix. Use one bucket per mail service
and an Object Read and Write token scoped to that bucket. Creating the
bucket needs an account level token, so create it in the Cloudflare
dashboard first.

## Bucket creation

The bucket is created automatically only when the key is allowed to. If
not, you get the exact policy above and the instruction to create it
yourself. No account wide credentials are ever requested.

## Settings

- `APP_MAIL_STORAGE_TIMEOUT_SECONDS`: per operation timeout, default 15.
- `APP_MAIL_STORAGE_ALLOW_PRIVATE`: set to `true` to allow a private
  endpoint such as a self-hosted MinIO. Off by default.
- `APP_MAIL_UNINSTALL_RETENTION_DAYS`: age before cleanup deletes data in
  the "after retention" uninstall mode, default 30.

## Uninstall cleanup

Deleting a service prefix needs the confirm token `delete <service-id>`.
"Now" removes everything under the prefix; "after retention" removes only
objects older than the retention window. Other services' prefixes are never
listed or touched.
