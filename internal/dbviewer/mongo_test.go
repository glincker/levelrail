package dbviewer

import (
	"encoding/json"
	"errors"
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
		{"out", MongoRequest{Op: MongoAggregate, Database: "d", Collection: "c", Pipeline: json.RawMessage(`[{"$out":"x"}]`)}},
		{"merge", MongoRequest{Op: MongoAggregate, Database: "d", Collection: "c", Pipeline: json.RawMessage(`[{"$match":{}},{"$merge":{"into":"x"}}]`)}},
		{"accumulator", MongoRequest{Op: MongoAggregate, Database: "d", Collection: "c", Pipeline: json.RawMessage(`[{"$group":{"_id":1,"v":{"$accumulator":{}}}}]`)}},
		{"out in lookup", MongoRequest{Op: MongoAggregate, Database: "d", Collection: "c", Pipeline: json.RawMessage(`[{"$lookup":{"from":"x","pipeline":[{"$out":"y"}],"as":"z"}}]`)}},
		{"two operator stage", MongoRequest{Op: MongoAggregate, Database: "d", Collection: "c", Pipeline: json.RawMessage(`[{"$match":{},"$limit":1}]`)}},
		{"js string filter", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`"function(){}"`)}},
		{"array filter", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`[1]`)}},
		{"not json", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{a:1}`)}},
		{"trailing", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(`{} ;db.x.drop()`)}},
		{"too large", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(big)}},
		{"too deep", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Filter: json.RawMessage(deep)}},
		{"negative limit", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Limit: -1}},
		{"bad sort", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Sort: json.RawMessage(`{"a":"x"}`)}},
		{"after with other sort", MongoRequest{Op: MongoFind, Database: "d", Collection: "c", Sort: json.RawMessage(`{"a":1}`), After: json.RawMessage(`"x"`)}},
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
