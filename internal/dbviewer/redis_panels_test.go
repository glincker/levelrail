package dbviewer

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestParseRedisInfoAllowList(t *testing.T) {
	in := strings.Split("# Server\nredis_version:7.2.4\nexecutable:/usr/local/bin/redis-server\nconfig_file:/etc/redis.conf\nrun_id:abc\ntcp_port:6379\n\n# Keyspace\ndb0:keys=3,expires=1,avg_ttl=0\n# Other\nx:1\n", "\n")
	info := ParseRedisInfo(in)
	if len(info.Sections) != 2 {
		t.Fatalf("sections=%+v", info.Sections)
	}
	for _, f := range info.Sections[0].Fields {
		if f.Key == "executable" || f.Key == "config_file" || f.Key == "run_id" {
			t.Errorf("leaked %s", f.Key)
		}
	}
	if len(info.Sections[0].Fields) != 2 || info.Sections[1].Fields[0].Key != "db0" {
		t.Fatalf("info=%+v", info)
	}
}

func slowEntryText(n int, id int, args []string) string {
	w := len(strconv.Itoa(n))
	var sb strings.Builder
	lab := func(i int) string { return strings.Repeat(" ", w-len(strconv.Itoa(i))) + strconv.Itoa(i) + ") " }
	pad := strings.Repeat(" ", w+2)
	return sb.String() + lab(id) + "1) (integer) " + strconv.Itoa(id) + "\n" +
		pad + "2) (integer) 1700000000\n" + pad + "3) (integer) 9\n" + pad + "4) 1) \"" + args[0] + "\"\n" +
		func() string {
			var o strings.Builder
			w2 := len(strconv.Itoa(len(args)))
			for i := 2; i <= len(args); i++ {
				o.WriteString(pad + "   " + strings.Repeat(" ", w2-len(strconv.Itoa(i))) + strconv.Itoa(i) + ") \"" + args[i-1] + "\"\n")
			}
			return o.String()
		}() + pad + "5) \"127.0.0.1:5000\"\n" + pad + "6) \"\"\n"
}

func TestParseRedisSlowlogManyArgsLeaksNone(t *testing.T) {
	args := []string{"hset", "k"}
	for i := 0; i < 12; i++ {
		args = append(args, "SECRET"+strconv.Itoa(i))
	}
	es := ParseRedisSlowlog(strings.Split(slowEntryText(1, 1, args), "\n"))
	if len(es) != 1 || es[0].Command != "HSET" || es[0].ArgCount != len(args)-1 || es[0].Client != "127.0.0.1:5000" {
		t.Fatalf("es=%+v", es)
	}
}

func TestParseRedisSlowlogManyEntries(t *testing.T) {
	for _, n := range []int{12, 120} {
		var sb strings.Builder
		for i := 1; i <= n; i++ {
			sb.WriteString(slowEntryText(n, i, []string{"set", "k", "v"}))
		}
		es := ParseRedisSlowlog(strings.Split(sb.String(), "\n"))
		if len(es) != n || es[0].ID != 1 || es[n-1].ID != int64(n) || es[n-1].Command != "SET" || es[n-1].ArgCount != 2 {
			t.Fatalf("n=%d got %d entries, first=%+v", n, len(es), es[0])
		}
	}
}

func TestParseRedisSlowlogRealOutput(t *testing.T) {
	b, err := os.ReadFile("testdata/slowlog_noraw.txt")
	if err != nil {
		t.Fatal(err)
	}
	es := ParseRedisSlowlog(strings.Split(string(b), "\n"))
	if len(es) != 12 || es[0].ID != 14 || es[0].Command != "SET" || es[0].ArgCount != 2 || es[11].Command != "SLOWLOG" {
		t.Fatalf("es=%+v", es)
	}
}

func TestParseRedisClientsDropsNames(t *testing.T) {
	cs := ParseRedisClients([]string{"id=4 addr=127.0.0.1:36992 fd=10 name=secret-name age=3 idle=1 flags=N db=0 cmd=client|list user=default", ""})
	if len(cs) != 1 || cs[0].Addr != "127.0.0.1:36992" || cs[0].Age != 3 || cs[0].User != "default" {
		t.Fatalf("cs=%+v", cs)
	}
}
