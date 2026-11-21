package fielddesc

// BackupTarget is the resource name of the backup destination form.
const BackupTarget = "backup-targets"

func init() {
	s3 := []Condition{{Field: "provider", Equals: []string{"aws", "r2", "custom"}}}
	custom := []Condition{{Field: "provider", Equals: []string{"custom"}}}
	r2custom := []Condition{{Field: "provider", Equals: []string{"r2", "custom"}}}
	dav := []Condition{{Field: "provider", Equals: []string{"webdav"}}}
	sftp := []Condition{{Field: "provider", Equals: []string{"sftp"}}}
	sftpKey := []Condition{{Field: "provider", Equals: []string{"sftp"}}, {Field: "auth", Equals: []string{"key"}}}
	sftpPass := []Condition{{Field: "provider", Equals: []string{"sftp"}}, {Field: "auth", Equals: []string{"password"}}}
	local := []Condition{{Field: "provider", Equals: []string{"local"}}}
	lb := func(id string) (string, string) {
		return "backups:fields.backupTarget." + id + ".label", "backups:fields.backupTarget." + id + ".help"
	}
	f := func(id, key string, t Type, req bool, show []Condition, flag, doc string) Field {
		l, h := lb(id)
		return Field{ID: id, Key: key, Type: t, LabelKey: l, HelpKey: h, Required: req, Secret: t == TypeSecret, ShowWhen: show, Flag: flag, Doc: doc}
	}
	name := f("name", "name", TypeString, true, nil, "name", "display name for the backup target")
	provider := f("provider", "provider", TypeSelect, true, nil, "provider", "provider: aws, r2, custom, local, webdav or sftp")
	provider.Options = []string{"aws", "r2", "custom", "local", "webdav", "sftp"}
	provider.Default = "aws"
	auth := f("sftp.auth", "auth", TypeSelect, true, sftp, "", "sftp authentication method, password or key (form only, the CLI infers it)")
	auth.Options = []string{"password", "key"}
	auth.Default = "password"

	Register(Resource{Name: BackupTarget, Fields: []Field{
		name,
		provider,
		f("s3.bucket", "bucket", TypeString, true, s3, "bucket", "bucket name"),
		f("s3.region", "region", TypeString, false, s3, "region", "bucket region (required for custom)"),
		f("s3.endpoint", "endpoint", TypeString, false, r2custom, "endpoint", "S3-compatible endpoint URL (required for r2 and custom)"),
		f("s3.access_key_id", "access_key_id", TypeString, true, s3, "access-key-id", "access key id"),
		f("s3.secret_access_key", "secret_access_key", TypeSecret, true, s3, "secret-access-key", "secret access key, write only"),
		f("custom.region", "region", TypeString, true, custom, "", "bucket region required by custom providers"),
		f("local.path", "bucket", TypeString, true, local, "path", "directory under the control plane local backup root"),
		f("webdav.endpoint", "endpoint", TypeString, true, dav, "", "https URL of the WebDAV share"),
		f("webdav.directory", "bucket", TypeString, true, dav, "", "remote directory on the share"),
		f("webdav.username", "access_key_id", TypeString, true, dav, "", "WebDAV username"),
		f("webdav.password", "secret_access_key", TypeSecret, true, dav, "", "WebDAV password, write only"),
		withFile(f("webdav.ca_bundle", "ca_bundle", TypeSecret, false, dav, "ca-bundle-file", "PEM file of extra CA certificates to trust, write only")),
		f("sftp.endpoint", "endpoint", TypeString, true, sftp, "", "SFTP server as HOST or HOST:PORT"),
		f("sftp.path", "bucket", TypeString, true, sftp, "", "remote directory, paths containing .. are refused"),
		f("sftp.username", "access_key_id", TypeString, true, sftp, "", "SSH username"),
		auth,
		f("sftp.password", "secret_access_key", TypeSecret, true, sftpPass, "", "SSH password, write only"),
		withFile(f("sftp.private_key", "private_key", TypeSecret, true, sftpKey, "private-key-file", "SSH private key file used instead of a password, write only")),
		f("sftp.private_key_passphrase", "private_key_passphrase", TypeSecret, false, sftpKey, "private-key-passphrase", "passphrase of an encrypted private key, write only"),
		f("sftp.host_key", "host_key", TypeString, true, sftp, "host-key", "pinned SSH host key as printed by scan-host-key"),
	}})
}

func withFile(f Field) Field {
	f.FlagFile = true
	return f
}
