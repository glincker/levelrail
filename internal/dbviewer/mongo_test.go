package dbviewer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func mongoTestLimits() Limits {
	return Limits{Timeout: 10 * time.Second, MaxRows: 200, MaxBytes: 1 << 20, MaxCellBytes: 4096, SchemaRows: 100, SQLBytes: 4096}
}

func TestMongoGuardRejects(t *testing.T) {
	big := `{"a":"` + strings.Repeat("x", 5000) + `"}`
	deep := strings.Repeat(`{"a":`, 30) + `1` + strings.Repeat(`}`, 30)
	cases := []struct {
		name string
		req  MongoRequest
	}{
		{"unknown op", MongoRequest{Op: "insertOne", Database: "d", Collection: "c"}},
		{"drop op", MongoRequest{Op: "dropDatabase", Database: "d"}},
		{"js in db name", MongoRequest{Op: MongoFind, Database: `d");db.dropDatabase();("`, Collection: "c"}},
		{"js in collection", MongoRequest{Op: MongoFind, Database: "d", Collection: "c');1"}},
		{"where", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"$where":"sleep(1000)"}`)}},
		{"nested where", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"a":{"$or":[{"$where":"1"}]}}`)}},
		{"function in expr", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"$expr":{"$function":{"body":"x","args":[],"lang":"js"}}}`)}},
		{"code type", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"a":{"$code":"1"}}`)}},
		{"js string filter", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`"function(){}"`)}},
		{"array filter", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`[1]`)}},
		{"not json", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{a:1}`)}},
		{"trailing", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{} ;db.x.drop()`)}},
		{"too large", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(big)}},
		{"too deep", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(deep)}},
		{"negative limit", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Limit: -1}},
		{"bad sort", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Sort: json.RawMessage(`{"a":"x"}`)}},
		{"after with other sort", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Sort: json.RawMessage(`{"a":1}`), After: json.RawMessage(`"x"`)}},
		{"admin system users", MongoRequest{Op: MongoFind, Database: "admin", Collection: "system.users"}},
		{"admin db find", MongoRequest{Op: MongoFind, Database: "admin", Collection: "c"}},
		{"config db", MongoRequest{Op: MongoCountDocuments, Database: "config", Collection: "c"}},
		{"local db", MongoRequest{Op: MongoGetIndexes, Database: "LOCAL", Collection: "c"}},
		{"system collection", MongoRequest{Op: MongoStats, Database: "app", Collection: "System.Profile"}},
		{"reserved db in overview", MongoRequest{Op: MongoCollections, Database: "admin"}},
		{"internal js emit", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"$_internalJsEmit":{"eval":"function(){while(1){}}","this":{},"fn":"x"}}`)}},
		{"nested internal", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"a":{"$and":[{"$_internalSchemaMinLength":1}]}}`)}},
		{"unknown operator", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"a":{"$jsonSchema":{}}}`)}},
		{"expr", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"$expr":{"$eq":[1,1]}}`)}},
		{"projection operator", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Projection: json.RawMessage(`{"a":{"$function":{}}}`)}},
		{"sort operator", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Sort: json.RawMessage(`{"$natural":1}`)}},
		{"long regex", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"a":{"$regex":"` + strings.Repeat("a", 300) + `"}}`)}},
		{"aggregate op", MongoRequest{Op: "aggregate", Database: "d", Collection: "c"}},
		{"after with operator", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", After: json.RawMessage(`{"$_x":1}`)}},
		{"stats without collection", MongoRequest{Op: MongoStats, Database: "d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CheckMongo(tc.req, mongoTestLimits()); !errors.Is(err, ErrMongoDenied) {
				t.Fatalf("err = %v, want ErrMongoDenied", err)
			}
		})
	}
}

func TestMongoGuardClampsLimit(t *testing.T) {
	q, err := CheckMongo(MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Limit: 1 << 30}, mongoTestLimits())
	if err != nil || q.Limit != 200 {
		t.Fatalf("limit = %d err = %v, want 200", q.Limit, err)
	}
}

func TestMongoFindScriptGolden(t *testing.T) {
	q, err := CheckMongo(MongoRequest{
		Op: MongoFind, Database: "app", Collection: "users", Limit: 25,
		Filter:     json.RawMessage(`{"age": {"$gt": 3}}`),
		Projection: json.RawMessage(`{"name":1}`),
		After:      json.RawMessage(`{"$oid":"650000000000000000000001"}`),
	}, mongoTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`var D=db.getSiblingDB("app");var C=D.getCollection("users");`,
		`var F=EJSON.parse("{\"age\":{\"$gt\":3}}");F={$and:[F,{_id:{$gt:EJSON.parse("{\"$oid\":\"650000000000000000000001\"}")}}]};`,
		`DRAIN(C.find(F,EJSON.parse("{\"name\":1}")).sort(EJSON.parse("{\"_id\":1}")).limit(L+1).maxTimeMS(MS));`,
		`var MS=10000,CELL=4096,L=25,SR=100;`,
	}
	for _, w := range want {
		if !strings.Contains(q.script, w) {
			t.Errorf("script missing %s\n%s", w, q.script)
		}
	}
	if strings.Contains(q.script, "MONGO_INITDB_ROOT_PASSWORD\"") && !strings.Contains(q.script, "process.env.MONGO_INITDB_ROOT_PASSWORD") {
		t.Error("credentials must come from the container env")
	}
}

func TestMongoFingerprintIgnoresValues(t *testing.T) {
	a, _ := CheckMongo(MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"a":1}`)}, mongoTestLimits())
	b, _ := CheckMongo(MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{"a":2}`)}, mongoTestLimits())
	if a.Fingerprint == "" || a.Fingerprint != b.Fingerprint {
		t.Fatalf("fingerprints differ: %q %q", a.Fingerprint, b.Fingerprint)
	}
}

func TestMongoGuardAccepts(t *testing.T) {
	ok := []MongoRequest{
		{Op: MongoListDatabases},
		{Op: MongoCollections},
		{Op: MongoFind, Database: "app", Collection: "users", Filter: json.RawMessage(`{"$or":[{"a":{"$in":[1,2]}},{"n":{"$regex":"^a","$options":"i"}}],"d":{"$gte":{"$date":"2020-01-01T00:00:00Z"}}}`), Projection: json.RawMessage(`{"a":{"$slice":2}}`)},
		{Op: MongoFind, Database: "app", Collection: "users", After: json.RawMessage(`{"$numberLong":"9007199254740993"}`), Sort: json.RawMessage(`{"_id":-1}`)},
	}
	for _, r := range ok {
		if _, err := CheckMongo(r, mongoTestLimits()); err != nil {
			t.Errorf("%s: %v", r.Op, err)
		}
	}
}

func TestMongoScriptHasDoneMarkerAndCanonicalCursor(t *testing.T) {
	q, err := CheckMongo(MongoRequest{Op: MongoFind, Database: "app", Collection: "c"}, mongoTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`print("LRDONE")`, `relaxed:false`, `"LRLAST:"`} {
		if !strings.Contains(q.script, w) {
			t.Errorf("script missing %s", w)
		}
	}
}

type mongoStubExec struct{ out string }

func (s mongoStubExec) ExecWithInput(_ context.Context, _ string, _ []string, _ io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.out)), nil
}

func TestRunMongoEndMarkerAndCursor(t *testing.T) {
	lim := mongoTestLimits()
	q, _ := CheckMongo(MongoRequest{Op: MongoFind, Database: "app", Collection: "c", Limit: 1}, lim)
	tg := Target{Exec: mongoStubExec{out: "{\"_id\":1}\n"}, Limits: lim}
	if _, err := tg.RunMongo(context.Background(), q); err == nil {
		t.Fatal("missing LRDONE must be an error")
	}
	tg.Exec = mongoStubExec{out: "{\"_id\":1}\nLRMORE\nLRLAST:{\"$numberLong\":\"9007199254740993\"}\nLRDONE\n"}
	res, err := tg.RunMongo(context.Background(), q)
	if err != nil || !res.Truncated || string(res.NextAfter) != `{"$numberLong":"9007199254740993"}` {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	tg.Exec = mongoStubExec{out: strings.Repeat("x", 200000) + "\n"}
	if res, err := tg.RunMongo(context.Background(), q); err != nil || !res.Truncated {
		t.Fatalf("oversized line must truncate, got %+v %v", res, err)
	}
}

func TestMongoCmdWrapsInTimeout(t *testing.T) {
	cmd := mongoCmd(15 * time.Second)
	if !strings.Contains(cmd[2], "exec timeout -k 2 15 mongosh") {
		t.Fatalf("cmd = %v", cmd)
	}
	if got := mongoCmd(1500 * time.Millisecond)[2]; !strings.Contains(got, "timeout -k 2 2 ") {
		t.Fatalf("cmd = %s", got)
	}
}

func TestMongoOverviewScriptIsBounded(t *testing.T) {
	q, err := CheckMongo(MongoRequest{Op: MongoCollections}, mongoTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`collStats:ci.name,maxTimeMS:MS`, `listCollections:1,nameOnly:true,maxTimeMS:MS`, `if(left<=0){return;}var d=`} {
		if !strings.Contains(q.script, w) {
			t.Errorf("overview script missing %s", w)
		}
	}
}

func TestRunMongoParsesLinesFromBytes(t *testing.T) {
	t.Parallel()
	out := "banner\n  {\"a\":1}  \nLRLAST:{\"$oid\":\"x\"}\n{\"a\":2}\nLRMORE\nLRDONE\n"
	tgt := Target{Exec: mongoStubExec{out: out}, Limits: Limits{Timeout: time.Second, MaxRows: 10, MaxBytes: 1 << 20, MaxCellBytes: 1024, SchemaRows: 10}}
	res, err := tgt.RunMongo(context.Background(), MongoQuery{Op: MongoFind, script: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Documents) != 2 || string(res.Documents[0]) != `{"a":1}` || !res.Truncated || string(res.NextAfter) != `{"$oid":"x"}` {
		t.Fatalf("res = %+v", res)
	}
}
