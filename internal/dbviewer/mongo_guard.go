package dbviewer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DialectMongo marks a MongoDB target. It is never a SQL dialect: only the
// typed Mongo requests below reach it.
const DialectMongo Dialect = "mongodb"

// MongoOp is one allow-listed read operation.
type MongoOp string

// Allowed operations. Anything else is rejected before a script is built.
const (
	MongoListDatabases   MongoOp = "listDatabases"
	MongoListCollections MongoOp = "listCollections"
	MongoCollections     MongoOp = "collections"
	MongoFind            MongoOp = "find"
	MongoCountDocuments  MongoOp = "countDocuments"
	MongoEstimatedCount  MongoOp = "estimatedDocumentCount"
	MongoGetIndexes      MongoOp = "getIndexes"
	MongoStats           MongoOp = "stats"
	mongoMaxDepth                = 16
	mongoMaxRegexLen             = 256
	mongoMaxSortKeys             = 4
)

// ErrMongoDenied reports a request the Mongo guard refuses.
var ErrMongoDenied = errors.New("mongodb request is not allowed")

var mongoNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,62}$`)

// Query, projection and extended JSON operators a request may use. Anything
// else, including every $_ internal and all JavaScript forms, is rejected.
// $where, $function, $accumulator, $out, $merge, $code and $expr stay out.
var mongoAllowKeys = map[string]bool{
	"$eq": true, "$ne": true, "$gt": true, "$gte": true, "$lt": true, "$lte": true,
	"$in": true, "$nin": true, "$and": true, "$or": true, "$nor": true, "$not": true,
	"$exists": true, "$type": true, "$mod": true, "$all": true, "$elemMatch": true,
	"$size": true, "$regex": true, "$options": true, "$slice": true,
	"$bitsAllSet": true, "$bitsAnySet": true, "$bitsAllClear": true, "$bitsAnyClear": true,
	"$oid": true, "$date": true, "$numberLong": true, "$numberInt": true,
	"$numberDouble": true, "$numberDecimal": true, "$binary": true, "$uuid": true,
	"$timestamp": true, "$t": true, "$i": true, "$regularExpression": true,
	"$minKey": true, "$maxKey": true, "$undefined": true,
}

// MongoRequest is the typed input; no field is ever executed as script text.
type MongoRequest struct {
	Op         MongoOp
	Database   string
	Collection string
	Filter     json.RawMessage
	Sort       json.RawMessage
	Projection json.RawMessage
	// After is the _id of the last document of the previous page.
	After json.RawMessage
	Limit int
}

// MongoQuery is a request the guard accepted, with its script ready to run.
type MongoQuery struct {
	Op          MongoOp
	Limit       int
	Fingerprint string
	script      string
}

// CheckMongo validates a request against the allow-list and the limits and
// builds the script from the re-serialised, validated data only.
func CheckMongo(req MongoRequest, lim Limits) (MongoQuery, error) {
	deny := func(format string, a ...any) (MongoQuery, error) {
		return MongoQuery{}, fmt.Errorf("%w: %s", ErrMongoDenied, fmt.Sprintf(format, a...))
	}
	switch req.Op {
	case MongoListDatabases, MongoCollections:
	case MongoListCollections, MongoStats:
		if !mongoNameRE.MatchString(req.Database) {
			return deny("invalid database name")
		}
		if req.Op == MongoStats && !mongoNameRE.MatchString(req.Collection) {
			return deny("invalid collection name")
		}
	case MongoFind, MongoCountDocuments, MongoEstimatedCount, MongoGetIndexes:
		if !mongoNameRE.MatchString(req.Database) || !mongoNameRE.MatchString(req.Collection) {
			return deny("invalid database or collection name")
		}
	default:
		return deny("operation %q", string(req.Op))
	}
	if req.Database != "" && !mongoNameRE.MatchString(req.Database) {
		return deny("invalid database name")
	}
	if req.Op != MongoListDatabases {
		if mongoReservedDB(req.Database) {
			return deny("system databases are not readable")
		}
		if mongoSystemCollection(req.Collection) {
			return deny("system collections are not readable")
		}
	}
	limit := req.Limit
	if limit < 0 {
		return deny("limit must not be negative")
	}
	if limit == 0 {
		limit = min(100, lim.MaxRows)
	}
	limit = min(limit, lim.MaxRows)

	filter, err := mongoObject(req.Filter, lim.SQLBytes)
	if err != nil {
		return deny("filter: %v", err)
	}
	proj, err := mongoObject(req.Projection, lim.SQLBytes)
	if err != nil {
		return deny("projection: %v", err)
	}
	sortSpec, err := mongoObject(req.Sort, lim.SQLBytes)
	if err != nil {
		return deny("sort: %v", err)
	}
	if err := mongoCheckSort(sortSpec); err != nil {
		return deny("sort: %v", err)
	}
	after, err := mongoValue(req.After, lim.SQLBytes)
	if err != nil {
		return deny("after: %v", err)
	}
	if after != "" && sortSpec != "{}" && !mongoSortIsID(sortSpec) {
		return deny("paging after a cursor requires sorting by _id only")
	}

	q := MongoQuery{Op: req.Op, Limit: limit}
	q.script = mongoScript(req, mongoParts{
		filter: filter, proj: proj, sort: sortSpec, after: after,
		limit: limit, lim: lim,
	})
	q.Fingerprint = mongoFingerprint(req, filter)
	return q, nil
}

// mongoParts holds validated, canonical JSON text for the script builder.
type mongoParts struct {
	filter, proj, sort, after string
	limit                     int
	lim                       Limits
}

func mongoDecode(raw json.RawMessage, maxBytes int) (any, error) {
	if len(raw) > maxBytes {
		return nil, errors.New("too large")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, errors.New("not valid JSON")
	}
	if dec.More() {
		return nil, errors.New("trailing data")
	}
	if err := mongoWalk(v, 0); err != nil {
		return nil, err
	}
	return v, nil
}

func mongoWalk(v any, depth int) error {
	if depth > mongoMaxDepth {
		return errors.New("nested too deeply")
	}
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if strings.HasPrefix(k, "$") && !mongoAllowKeys[k] {
				return fmt.Errorf("operator %s is not allowed", k)
			}
			if k == "$regex" || k == "$pattern" || k == "pattern" {
				if p, ok := child.(string); ok && len(p) > mongoMaxRegexLen {
					return errors.New("regular expression is too long")
				}
			}
			if err := mongoWalk(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range x {
			if err := mongoWalk(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func mongoReservedDB(name string) bool {
	switch strings.ToLower(name) {
	case "admin", "config", "local":
		return true
	}
	return false
}

func mongoSystemCollection(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "system.")
}

// mongoObject returns canonical JSON for an optional JSON object, "{}" if empty.
func mongoObject(raw json.RawMessage, maxBytes int) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "{}", nil
	}
	v, err := mongoDecode(raw, maxBytes)
	if err != nil {
		return "", err
	}
	if _, ok := v.(map[string]any); !ok {
		return "", errors.New("must be a JSON object")
	}
	out, _ := json.Marshal(v)
	return string(out), nil
}

func mongoValue(raw json.RawMessage, maxBytes int) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil
	}
	v, err := mongoDecode(raw, maxBytes)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(v)
	return string(out), nil
}

func mongoCheckSort(canonical string) error {
	var m map[string]json.Number
	dec := json.NewDecoder(strings.NewReader(canonical))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return errors.New("values must be 1 or -1")
	}
	if len(m) > mongoMaxSortKeys {
		return fmt.Errorf("at most %d keys", mongoMaxSortKeys)
	}
	for k, d := range m {
		if strings.HasPrefix(k, "$") {
			return errors.New("operators are not allowed")
		}
		if d.String() != "1" && d.String() != "-1" {
			return errors.New("values must be 1 or -1")
		}
	}
	return nil
}

func mongoSortIsID(canonical string) bool {
	return canonical == `{"_id":1}` || canonical == `{"_id":-1}`
}

// mongoFingerprint hashes the operation and the shape of the request with
// every literal value replaced, so equal queries share one audit id.
func mongoFingerprint(req MongoRequest, filter string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%s|", req.Op, req.Database, req.Collection)
	for _, s := range []string{filter} {
		var v any
		_ = json.Unmarshal([]byte(s), &v)
		b.WriteString(mongoShape(v))
		b.WriteByte('|')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}

func mongoShape(v any) string {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for _, k := range keys {
			b.WriteString(k + ":" + mongoShape(x[k]) + ",")
		}
		b.WriteByte('}')
		return b.String()
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for _, c := range x {
			b.WriteString(mongoShape(c) + ",")
		}
		b.WriteByte(']')
		return b.String()
	}
	return "?"
}
