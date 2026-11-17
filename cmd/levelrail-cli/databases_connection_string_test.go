package main

import "testing"

func TestConnectionStringPerEngine(t *testing.T) {
	cases := []struct {
		name   string
		target connStringTarget
		format string
		want   string
	}{
		{"postgres tls", connStringTarget{engine: "postgres", user: "postgres", password: "p@ss/w", host: "db", port: 5432, database: "app", tls: true}, "url", "postgres://postgres:p%40ss%2Fw@db:5432/app?sslmode=require"},
		{"postgres masked env", connStringTarget{engine: "postgres", user: "u", password: connectionStringMask, host: "db", port: 5432, database: "app"}, "env", "DATABASE_URL=postgres://u:xxxxxxxx@db:5432/app?sslmode=disable"},
		{"mysql", connStringTarget{engine: "mysql", user: "root", password: "pw", host: "db", port: 3306, database: "shop"}, "url", "mysql://root:pw@db:3306/shop"},
		{"mariadb", connStringTarget{engine: "mariadb", user: "root", password: "pw", host: "db", port: 3306}, "url", "mysql://root:pw@db:3306/"},
		{"redis", connStringTarget{engine: "redis", password: "pw", host: "cache", port: 6379}, "url", "redis://:pw@cache:6379"},
		{"redis tls", connStringTarget{engine: "valkey", password: "pw", host: "cache", port: 6379, tls: true}, "url", "rediss://:pw@cache:6379"},
		{"mongodb", connStringTarget{engine: "mongodb", user: "root", password: "pw", host: "m", port: 27017, database: "admin"}, "url", "mongodb://root:pw@m:27017/admin"},
		{"json", connStringTarget{engine: "mysql", user: "root", password: "pw", host: "db", port: 3306, database: "shop"}, "json", `{"database":"shop","engine":"mysql","host":"db","password":"pw","port":"3306","url":"mysql://root:pw@db:3306/shop","user":"root"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderConnectionString(tc.target, tc.format)
			if err != nil || got != tc.want {
				t.Fatalf("got %q err %v, want %q", got, err, tc.want)
			}
		})
	}
	if _, err := renderConnectionString(connStringTarget{engine: "clickhouse"}, "url"); err == nil {
		t.Fatal("unknown engine must error")
	}
}
