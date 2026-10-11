package appwizard

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Engine ids, matching internal/store's database engine registry.
const (
	enginePostgres = "postgres"
	engineRedis    = "redis"
	engineMySQL    = "mysql"
	engineMongo    = "mongodb"
	engineMariaDB  = "mariadb"

	fieldURL = "url"
)

var (
	keyEngineRe = []struct {
		re     *regexp.Regexp
		engine string
	}{
		{regexp.MustCompile(`^(POSTGRES(QL)?_.*|PG(HOST|USER|PASSWORD|DATABASE|PORT)|POSTGRES_URL)$`), enginePostgres},
		{regexp.MustCompile(`^(REDIS_.*|REDISCLOUD_URL)$`), engineRedis},
		{regexp.MustCompile(`^(MYSQL_.*|MARIADB_.*)$`), engineMySQL},
		{regexp.MustCompile(`^(MONGO(DB)?_.*)$`), engineMongo},
	}
	genericDBKeyRe = regexp.MustCompile(`^(DATABASE_URL|DB_URL|DATABASE_CONNECTION_STRING)$`)

	depEngines = []struct {
		engine string
		tokens []string
	}{
		{enginePostgres, []string{`"pg"`, "psycopg", "asyncpg", "jackc/pgx", "lib/pq", "gem 'pg'", `gem "pg"`, "postgres", "pdo_pgsql"}},
		{engineMySQL, []string{"mysql2", "mysqlclient", "pymysql", "go-sql-driver/mysql", "gem 'mysql2'", `gem "mysql2"`, "pdo_mysql"}},
		{engineMongo, []string{"mongoose", `"mongodb"`, "pymongo", "mongo-driver", "mongoid"}},
		{engineRedis, []string{`"redis"`, "ioredis", "bullmq", "go-redis", "redis-py", "\nredis", "gem 'redis'", `gem "redis"`, "sidekiq"}},
	}

	imageTagRe = regexp.MustCompile(`^(\d+)(?:[.\-].*)?$`)
)

var engineDefaultEnvVar = map[string]string{
	enginePostgres: "DATABASE_URL",
	engineMySQL:    "DATABASE_URL",
	engineRedis:    "REDIS_URL",
	engineMongo:    "MONGODB_URI",
	engineMariaDB:  "DATABASE_URL",
}

// engineForImage maps a container image to a managed engine id.
func engineForImage(image string) (string, bool) {
	name := strings.ToLower(image)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name, _, _ = strings.Cut(name, ":")
	switch name {
	case "postgres", "postgis", "pgvector":
		return enginePostgres, true
	case "redis", "valkey":
		return engineRedis, true
	case "mysql":
		return engineMySQL, true
	case "mariadb":
		return engineMariaDB, true
	case "mongo", "mongodb":
		return engineMongo, true
	}
	return "", false
}

func imageMajor(image string) string {
	_, tag, ok := strings.Cut(image, ":")
	if !ok {
		return ""
	}
	if m := imageTagRe.FindStringSubmatch(tag); m != nil {
		return m[1]
	}
	return ""
}

type dbSignals struct {
	keys    []EnvKey
	depText string
	compose composeDoc
	cpath   string
}

// detectDatabases returns at most one need per engine, with evidence.
func detectDatabases(appName string, sig dbSignals, defaultVersion func(string) string) []DatabaseNeed {
	needs := map[string]*DatabaseNeed{}
	add := func(engine, envVar, evidence, version string) {
		if _, ok := needs[engine]; ok {
			return
		}
		v := version
		if v == "" && defaultVersion != nil {
			v = defaultVersion(engine)
		}
		if envVar == "" {
			envVar = engineDefaultEnvVar[engine]
		}
		needs[engine] = &DatabaseNeed{
			Engine: engine, Version: v, SuggestedName: appName + "-" + engine,
			EnvVar: envVar, Field: fieldURL, Evidence: evidence,
		}
	}
	depHit := func(engine string) string {
		for _, d := range depEngines {
			if d.engine != engine {
				continue
			}
			for _, t := range d.tokens {
				if strings.Contains(sig.depText, t) {
					return strings.Trim(strings.TrimSpace(t), `"'`)
				}
			}
		}
		return ""
	}

	for _, k := range sig.keys {
		for _, ke := range keyEngineRe {
			if ke.re.MatchString(k.Key) {
				add(ke.engine, urlKeyFor(ke.engine, k.Key), fmt.Sprintf("%s declared in %s", k.Key, k.Source), "")
			}
		}
	}
	for _, k := range sig.keys {
		if !genericDBKeyRe.MatchString(k.Key) {
			continue
		}
		engine, why := enginePostgres, "assumed Postgres, the most common choice"
		for _, e := range []string{enginePostgres, engineMySQL, engineMongo} {
			if hit := depHit(e); hit != "" {
				engine, why = e, hit+" found in the dependency manifest"
				break
			}
		}
		add(engine, k.Key, fmt.Sprintf("%s declared in %s, %s", k.Key, k.Source, why), "")
	}
	for _, e := range []string{engineRedis, enginePostgres, engineMySQL, engineMongo} {
		if hit := depHit(e); hit != "" {
			add(e, "", hit+" found in the dependency manifest", "")
		}
	}
	names := make([]string, 0, len(sig.compose.Services))
	for n := range sig.compose.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		img := sig.compose.Services[n].Image
		if engine, ok := engineForImage(img); ok {
			add(engine, "", fmt.Sprintf("%s runs %s as service %q", sig.cpath, img, n), imageMajor(img))
		}
	}

	out := make([]DatabaseNeed, 0, len(needs))
	for _, n := range needs {
		out = append(out, *n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Engine < out[j].Engine })
	return out
}

// urlKeyFor chooses the env var the connection URL should land in: the key
// itself when it already looks like a URL, else the engine's conventional one.
func urlKeyFor(engine, key string) string {
	if strings.HasSuffix(key, "_URL") || strings.HasSuffix(key, "_URI") {
		return key
	}
	return engineDefaultEnvVar[engine]
}

// markProvided flags env keys a suggested database will fill in, so the
// wizard does not ask for them.
func markProvided(keys []EnvKey, dbs []DatabaseNeed) {
	for i := range keys {
		for _, d := range dbs {
			if keys[i].Key == d.EnvVar {
				keys[i].ProvidedBy = "database:" + d.SuggestedName
				keys[i].Required = false
			}
		}
	}
}
