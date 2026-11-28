package extdb

import (
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/mysqlssl"
)

func TestParseURL_Providers(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want Parsed
	}{
		{"neon upgrades require to verify-full", "postgres://ada:p%40ss@ep-cool-123456.eu-central-1.aws.neon.tech/appdb?sslmode=require",
			Parsed{Engine: EnginePostgres, Host: "ep-cool-123456.eu-central-1.aws.neon.tech", Port: 5432, User: "ada", Password: "p@ss", Database: "appdb", TLSMode: TLSVerifyFull, Provider: "neon"}},
		{"rds postgres", "postgresql://master:pw@db1.abc123.us-east-1.rds.amazonaws.com:5432/postgres",
			Parsed{Engine: EnginePostgres, Host: "db1.abc123.us-east-1.rds.amazonaws.com", Port: 5432, User: "master", Password: "pw", Database: "postgres", TLSMode: TLSRequire, Provider: "aws-rds"}},
		{"aurora cluster endpoint", "mysql://admin:pw@prod.cluster-abc123.us-east-1.rds.amazonaws.com/shop",
			Parsed{Engine: EngineMySQL, Host: "prod.cluster-abc123.us-east-1.rds.amazonaws.com", Port: 3306, User: "admin", Password: "pw", Database: "shop", TLSMode: TLSRequire, Provider: "aws-aurora"}},
		{"supabase", "postgres://postgres:pw@db.abcdefgh.supabase.co:6543/postgres",
			Parsed{Engine: EnginePostgres, Host: "db.abcdefgh.supabase.co", Port: 6543, User: "postgres", Password: "pw", Database: "postgres", TLSMode: TLSRequire, Provider: "supabase"}},
		{"planetscale", "mysql://u:pw2@aws.connect.psdb.cloud/app?ssl-mode=REQUIRED",
			Parsed{Engine: EngineMySQL, Host: "aws.connect.psdb.cloud", Port: 3306, User: "u", Password: "pw2", Database: "app", TLSMode: TLSVerifyFull, Provider: "planetscale"}},
		{"atlas srv has no port", "mongodb+srv://u:pw@cluster0.ab1cd.mongodb.net/shop?authSource=admin&retryWrites=true",
			Parsed{Engine: EngineMongoDB, Host: "cluster0.ab1cd.mongodb.net", User: "u", Password: "pw", Database: "shop", AuthDatabase: "admin", TLSMode: TLSVerifyFull, Provider: "mongodb-atlas"}},
		{"mongodb seed list keeps first host", "mongodb://u:pw@h1.example.net:27017,h2.example.net:27017/app?tls=true",
			Parsed{Engine: EngineMongoDB, Host: "h1.example.net", Port: 27017, User: "u", Password: "pw", Database: "app", TLSMode: TLSRequire}},
		{"upstash rediss", "rediss://default:tok@eu1-calm-12345.upstash.io:6379",
			Parsed{Engine: EngineRedis, Host: "eu1-calm-12345.upstash.io", Port: 6379, User: "default", Password: "tok", TLSMode: TLSVerifyFull, Provider: "upstash"}},
		{"plain redis stays unencrypted", "redis://:pw@cache.internal.example.com",
			Parsed{Engine: EngineRedis, Host: "cache.internal.example.com", Port: 6379, Password: "pw", TLSMode: TLSDisable}},
		{"azure", "postgres://u%40srv:pw@srv.postgres.database.azure.com/db?sslmode=verify-full",
			Parsed{Engine: EnginePostgres, Host: "srv.postgres.database.azure.com", Port: 5432, User: "u@srv", Password: "pw", Database: "db", TLSMode: TLSVerifyFull, Provider: "azure"}},
		{"explicit disable is kept", "postgres://u:pw@db.abc.supabase.co/x?sslmode=disable",
			Parsed{Engine: EnginePostgres, Host: "db.abc.supabase.co", Port: 5432, User: "u", Password: "pw", Database: "x", TLSMode: TLSDisable, Provider: "supabase"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseURL(tc.raw)
			if err != nil {
				t.Fatalf("ParseURL: %v", err)
			}
			if got != tc.want {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestParseURL_RejectsWithoutEchoingInput(t *testing.T) {
	for _, raw := range []string{"", "http://u:secretpw@host/db", "postgres://u:secretpw@/db", "postgres://u:secretpw@h:99999/db", "ftp://x", "not a url secretpw"} {
		_, err := ParseURL(raw)
		if err == nil {
			t.Errorf("%q parsed, want an error", raw)
			continue
		}
		if strings.Contains(err.Error(), "secretpw") {
			t.Errorf("error leaks the input: %v", err)
		}
	}
}

func TestDetectProvider(t *testing.T) {
	cases := map[string]string{
		"ep-a-1.us-east-2.aws.neon.tech":               "neon",
		"x.abc.us-east-1.rds.amazonaws.com":            "aws-rds",
		"x.cluster-abc.us-east-1.rds.amazonaws.com":    "aws-aurora",
		"x.cluster-ro-abc.eu-west-1.rds.amazonaws.com": "aws-aurora",
		"db.ref.supabase.co":                           "supabase",
		"aws-0-eu.pooler.supabase.com":                 "supabase",
		"aws.connect.psdb.cloud":                       "planetscale",
		"c0.ab.mongodb.net":                            "mongodb-atlas",
		"eu1-x-1.upstash.io":                           "upstash",
		"s.mysql.database.azure.com":                   "azure",
		"inst.europe-west1.sql.goog":                   "gcp-cloudsql",
		"pg-1-proj.aivencloud.com":                     "aiven",
		"db.example.com":                               "",
		"notneon.tech":                                 "",
		"evilneon.tech.example.com":                    "",
	}
	for host, want := range cases {
		if got := DetectProvider(host); got != want {
			t.Errorf("DetectProvider(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestProviders_PresetsAreValid(t *testing.T) {
	for _, p := range Providers() {
		for _, e := range p.Engines {
			if !Supported(e) {
				t.Errorf("%s lists unsupported engine %s", p.ID, e)
			}
		}
		c := Conn{Engine: p.Engines[0], Host: "db.example.com", TLSMode: p.TLSMode}
		if err := c.Validate(Policy{}); err != nil {
			t.Errorf("%s preset TLS mode invalid: %v", p.ID, err)
		}
	}
}

func TestValidate_RejectsOptionLikeIdentifiers(t *testing.T) {
	for _, v := range []string{"-h169.254.169.254", "-eSHUTDOWN", "--skip-ssl", "-"} {
		for _, field := range []string{"user", "database", "auth"} {
			c := Conn{Engine: EngineMySQL, Host: "db.example.com"}
			switch field {
			case "user":
				c.User = v
			case "database":
				c.Database = v
			default:
				c.AuthDatabase = v
			}
			if err := c.Validate(Policy{}); err == nil {
				t.Errorf("%s %q accepted", field, v)
			}
		}
	}
	ok := Conn{Engine: EngineMySQL, Host: "db.example.com", User: "app-user", Database: "my-db"}
	if err := ok.Validate(Policy{}); err != nil {
		t.Errorf("plain identifiers rejected: %v", err)
	}
}

func TestValidate_HostForms(t *testing.T) {
	blocked := []string{"2852039166", "0xa9fea9fe", "0251.0376.0251.0376", "169.254.43518", "0xA9.0xFE.0xA9.0xFE", "169.254.169.254", "64:ff9b::a9fe:a9fe", "[64:ff9b::1]", "metadata.google.internal", "metadata", "127.1", "localhost", "0"}
	for _, h := range blocked {
		c := Conn{Engine: EnginePostgres, Host: h}
		if err := c.Validate(Policy{}); err == nil {
			t.Errorf("host %q accepted", h)
		}
	}
	for _, h := range []string{"db.example.com", "10.1.2.3", "pg-1", "2001-db8.example.com", "1password.example.com"} {
		c := Conn{Engine: EnginePostgres, Host: h}
		if err := c.Validate(Policy{}); err != nil {
			t.Errorf("host %q rejected: %v", h, err)
		}
	}
}

func TestParseURL_TLSExplicitOffAndPresets(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"redis://:pw@eu1-x-1.upstash.io:6379", TLSVerifyFull},
		{"mongodb://u:pw@c0.ab.mongodb.net/x", TLSVerifyFull},
		{"mongodb://u:pw@c0.ab.mongodb.net/x?tls=false", TLSDisable},
		{"mysql://u:pw@aws.connect.psdb.cloud/x?ssl-mode=DISABLED", TLSDisable},
		{"mysql://u:pw@aws.connect.psdb.cloud/x?ssl=false", TLSDisable},
		{"redis://:pw@cache.internal.example.com", TLSDisable},
		{"postgres://u:pw@db.example.com/x?sslmode=disable", TLSDisable},
	}
	for _, tc := range cases {
		got, err := ParseURL(tc.raw)
		if err != nil || got.TLSMode != tc.want {
			t.Errorf("%s: tls = %q err %v, want %q", tc.raw, got.TLSMode, err, tc.want)
		}
	}
	if PresetTLSMode("neon") != TLSVerifyFull || PresetTLSMode(ProviderGeneric) != "" || PresetTLSMode("") != "" {
		t.Error("preset TLS lookup wrong")
	}
}

func TestClientTLSScripts(t *testing.T) {
	my := mysqlssl.Case(false)
	for _, want := range []string{"VERIFY_IDENTITY --ssl-ca=$CA", "VERIFY_CA --ssl-ca=$CA", "/etc/pki/tls/certs/ca-bundle.crt"} {
		if !strings.Contains(my, want) {
			t.Errorf("mysql case missing %q", want)
		}
	}
	if maria := mysqlssl.Case(true); !strings.Contains(maria, "--ssl-verify-server-cert --ssl-ca=$CA") {
		t.Error("mariadb verify mode lacks the CA file")
	}
	mongo, _ := ProbeScript(EngineMongoDB)
	if !strings.Contains(mongo, `mongodb+srv://$EXT_HOST/`) || strings.Contains(mongo, "--tlsAllowInvalidCertificates;; verify-full") {
		t.Errorf("mongo script: %s", mongo)
	}
	for _, mode := range []string{TLSVerifyCA, TLSVerifyFull} {
		env := HelperEnv(Conn{Engine: EnginePostgres, Host: "h", Port: 5432, TLSMode: mode})
		if env["PGSSLROOTCERT"] != "/etc/ssl/certs/ca-certificates.crt" || env["PGSSLMODE"] != mode {
			t.Errorf("%s env = %v", mode, env)
		}
	}
	if HelperEnv(Conn{Engine: EngineMongoDB, Host: "c0.ab.mongodb.net", Port: 27017})["EXT_SRV"] != "1" {
		t.Error("atlas host should use the seed record form")
	}
	if _, ok := HelperEnv(Conn{Engine: EngineMongoDB, Host: "db.example.com", Port: 27017})["EXT_SRV"]; ok {
		t.Error("plain mongo host must not use the seed record form")
	}
}
