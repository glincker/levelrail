package database

import (
	"archive/tar"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMySQLCommand_BinlogFlagsOnlyWhenPITREnabled(t *testing.T) {
	t.Setenv(EnvBinlogExpireSeconds, "")
	off := mysqlCommand(defaultSlowQueryThresholdMs, nil, false)
	for _, f := range off {
		if strings.HasPrefix(f, "--log-bin") {
			t.Fatalf("binlog flag %q present without PITR: %v", f, off)
		}
	}
	on := mysqlCommand(defaultSlowQueryThresholdMs, nil, true)
	for _, want := range []string{
		"--log-bin=" + MySQLBinlogPath + "/" + MySQLBinlogBasename,
		"--binlog-format=ROW",
		"--binlog-expire-logs-seconds=604800",
	} {
		if !contains(on, want) {
			t.Errorf("command %v is missing %q", on, want)
		}
	}
}

func TestMySQLCommand_ExpiryComesFromEnv(t *testing.T) {
	t.Setenv(EnvBinlogExpireSeconds, "3600")
	if got := mysqlCommand(defaultSlowQueryThresholdMs, nil, true); !contains(got, "--binlog-expire-logs-seconds=3600") {
		t.Errorf("command %v ignores %s", got, EnvBinlogExpireSeconds)
	}
	t.Setenv(EnvBinlogExpireSeconds, "-4")
	if got := mysqlCommand(defaultSlowQueryThresholdMs, nil, true); !contains(got, "--binlog-expire-logs-seconds=604800") {
		t.Errorf("command %v accepted an invalid %s", got, EnvBinlogExpireSeconds)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestSupportsPITR(t *testing.T) {
	for engine, want := range map[string]bool{
		store.EnginePostgres: true, store.EngineMySQL: true, store.EngineMariaDB: true,
		store.EngineMongoDB: false, store.EngineRedis: false,
	} {
		if got := SupportsPITR(engine); got != want {
			t.Errorf("SupportsPITR(%q) = %v, want %v", engine, got, want)
		}
	}
}

func TestController_Reconcile_MySQLFamily_PITR_MountsBinlogVolumeOwnedByServerUser(t *testing.T) {
	for _, engine := range []string{store.EngineMySQL, store.EngineMariaDB} {
		t.Run(engine, func(t *testing.T) {
			rt := newFakeRuntime()
			desired := &store.DesiredDatabase{Name: "main", Engine: engine, Version: "8.4", PITREnabled: true}
			opt := WithMySQLCredentials(&MySQLCredentials{Username: "u", Password: "p"})
			if engine == store.EngineMariaDB {
				opt = WithMariaDBCredentials(&MariaDBCredentials{Username: "u", Password: "p"})
			}
			c := New("main", &fakeStore{db: desired}, rt, opt)

			result, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition = %+v, want ready", cond)
			}

			vol := walArchiveVolumeName("main")
			mounted := false
			for _, m := range rt.lastCreateSpec.Volumes {
				if m.Name == vol && m.ContainerPath == MySQLBinlogPath {
					mounted = true
				}
			}
			if !mounted {
				t.Errorf("volumes %+v have no %q mount at %s", rt.lastCreateSpec.Volumes, vol, MySQLBinlogPath)
			}
			if rt.lastCreateSpec.Labels[pitrLabelKey] != "true" {
				t.Errorf("pitr label = %q, want true", rt.lastCreateSpec.Labels[pitrLabelKey])
			}
			if !contains(rt.lastCreateSpec.Command, "--binlog-format=ROW") {
				t.Errorf("command %v has no binlog flags", rt.lastCreateSpec.Command)
			}
			chowned := false
			for _, cmd := range rt.execArgCmds {
				if reflect.DeepEqual(cmd, []string{"chown", "-R", "999:999", certsHelperMountPath}) {
					chowned = true
				}
			}
			if !chowned {
				t.Errorf("exec calls %v never chowned the binlog volume before the server first mounted it", rt.execArgCmds)
			}
		})
	}
}

func TestController_Reconcile_MySQL_NoPITR_LeavesCommandUnchanged(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8.4"}
	c := New("main", &fakeStore{db: desired}, rt, WithMySQLCredentials(&MySQLCredentials{Username: "u", Password: "p"}))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.hasVolume(walArchiveVolumeName("main")) {
		t.Error("a database without PITR got a binlog volume")
	}
	if contains(rt.lastCreateSpec.Command, "--binlog-format=ROW") {
		t.Errorf("command %v enables the binlog without PITR", rt.lastCreateSpec.Command)
	}
}

func TestMongoCommand_TLSOnlyWithMaterial(t *testing.T) {
	if got := mongoCommand(nil); got != nil {
		t.Errorf("mongoCommand(nil) = %v, want nil so existing databases are unchanged", got)
	}
	got := mongoCommand(&TLSMaterial{})
	for _, want := range []string{"preferTLS", certsMountPath + "/" + tlsPEMFile, certsMountPath + "/" + tlsCertFile} {
		if !contains(got, want) {
			t.Errorf("mongoCommand = %v, missing %q", got, want)
		}
	}
}

func TestTLSTarArchive_IncludesCombinedPEMOwnedByServerUser(t *testing.T) {
	buf, err := tlsTarArchive(&TLSMaterial{CertPEM: []byte("CERT"), KeyPEM: []byte("KEY")})
	if err != nil {
		t.Fatalf("tlsTarArchive() error = %v", err)
	}
	tr := tar.NewReader(buf)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("archive has no %s", tlsPEMFile)
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		if h.Name != tlsPEMFile {
			continue
		}
		body, _ := io.ReadAll(tr)
		if string(body) != "KEYCERT" || h.Mode != 0o600 || h.Uid != dbTLSUID || h.Gid != dbTLSGID {
			t.Errorf("%s = %q mode %o uid %d gid %d", tlsPEMFile, body, h.Mode, h.Uid, h.Gid)
		}
		return
	}
}
