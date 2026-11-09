package api

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// connectSnippetPlaceholder stands in for the password whenever the caller may not see it.
const connectSnippetPlaceholder = "<password>"

// ConnectSnippetTarget is the endpoint a snippet connects to.
type ConnectSnippetTarget struct {
	Engine   string
	Host     string
	Port     int
	User     string
	Database string
	TLS      bool
}

// ConnectSnippet is one copyable example for a language or client tool.
type ConnectSnippet struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Code  string `json:"code"`
}

// BuildConnectSnippets renders examples per language and tool. The password is only
// embedded when reveal is true, so read-only roles get a placeholder.
func BuildConnectSnippets(t ConnectSnippetTarget, password string, reveal bool) []ConnectSnippet {
	pw := connectSnippetPlaceholder
	if reveal && password != "" {
		pw = password
	}
	addr := net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	switch t.Engine {
	case "postgres":
		u := snippetURL("postgresql", t, pw, addr, t.Database, map[string]string{"sslmode": sslMode(t.TLS)})
		return []ConnectSnippet{
			{"url", "Connection URL", u},
			{"psql", "psql", fmt.Sprintf("PGPASSWORD=%s psql -h %s -p %d -U %s -d %s", shellQuote(pw), t.Host, t.Port, t.User, t.Database)},
			{"go", "Go (pgx)", fmt.Sprintf("conn, err := pgx.Connect(ctx, %q)", u)},
			{"node", "Node.js (pg)", fmt.Sprintf("const client = new Client({ connectionString: %q });\nawait client.connect();", u)},
			{"python", "Python (psycopg)", fmt.Sprintf("conn = psycopg.connect(%q)", u)},
		}
	case "mysql", "mariadb":
		u := snippetURL("mysql", t, pw, addr, t.Database, nil)
		return []ConnectSnippet{
			{"url", "Connection URL", u},
			{"cli", "mysql", fmt.Sprintf("MYSQL_PWD=%s mysql -h %s -P %d -u %s %s", shellQuote(pw), t.Host, t.Port, t.User, t.Database)},
			{"go", "Go (database/sql)", fmt.Sprintf("db, err := sql.Open(\"mysql\", %q)", fmt.Sprintf("%s:%s@tcp(%s)/%s", t.User, pw, addr, t.Database))},
			{"node", "Node.js (mysql2)", fmt.Sprintf("const conn = await mysql.createConnection(%q);", u)},
			{"python", "Python (PyMySQL)", fmt.Sprintf("conn = pymysql.connect(host=%q, port=%d, user=%q, password=%q, database=%q)", t.Host, t.Port, t.User, pw, t.Database)},
		}
	case "redis", "valkey", "keydb":
		scheme := "redis"
		if t.TLS {
			scheme = "rediss"
		}
		u := snippetURL(scheme, t, pw, addr, "", nil)
		return []ConnectSnippet{
			{"url", "Connection URL", u},
			{"cli", "redis-cli", fmt.Sprintf("REDISCLI_AUTH=%s redis-cli -h %s -p %d%s", shellQuote(pw), t.Host, t.Port, tlsFlag(t.TLS))},
			{"go", "Go (go-redis)", fmt.Sprintf("opt, err := redis.ParseURL(%q)", u)},
			{"node", "Node.js (ioredis)", fmt.Sprintf("const redis = new Redis(%q);", u)},
			{"python", "Python (redis-py)", fmt.Sprintf("r = redis.Redis.from_url(%q)", u)},
		}
	case "mongodb":
		u := snippetURL("mongodb", t, pw, addr, t.Database, nil)
		return []ConnectSnippet{
			{"url", "Connection URL", u},
			{"cli", "mongosh", fmt.Sprintf("mongosh %q", u)},
			{"node", "Node.js (mongodb)", fmt.Sprintf("const client = new MongoClient(%q);", u)},
			{"python", "Python (pymongo)", fmt.Sprintf("client = MongoClient(%q)", u)},
		}
	}
	return nil
}

func snippetURL(scheme string, t ConnectSnippetTarget, pw, addr, path string, q map[string]string) string {
	u := url.URL{Scheme: scheme, Host: addr}
	if t.User != "" {
		u.User = url.UserPassword(t.User, pw)
	} else {
		u.User = url.UserPassword("", pw)
	}
	if path != "" {
		u.Path = "/" + path
	}
	if len(q) > 0 {
		vals := url.Values{}
		for k, v := range q {
			vals.Set(k, v)
		}
		u.RawQuery = vals.Encode()
	}
	// url.String escapes the placeholder brackets; keep it readable.
	return unescapePlaceholder(u.String())
}

func unescapePlaceholder(s string) string {
	return strings.ReplaceAll(s, "%3Cpassword%3E", connectSnippetPlaceholder)
}

func sslMode(tls bool) string {
	if tls {
		return "require"
	}
	return "disable"
}

func tlsFlag(tls bool) string {
	if tls {
		return " --tls"
	}
	return ""
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
