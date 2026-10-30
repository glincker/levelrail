// Package mailstorage holds the per-mail-service bring-your-own-bucket
// core: sealed connections, key layout, probe, bucket creation and cleanup.
// It has no global lookup, so one service can never reach another's bucket.
package mailstorage
