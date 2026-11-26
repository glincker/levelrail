package dbviewer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DynamoOp is one allow-listed read operation. Writes and table changes have no constant here, so they cannot be built.
type DynamoOp string

// Allowed operations; the value is the DynamoDB API action name.
const (
	DynamoListTables       DynamoOp = "ListTables"
	DynamoDescribeTable    DynamoOp = "DescribeTable"
	DynamoDescribeTTL      DynamoOp = "DescribeTimeToLive"
	DynamoQuery            DynamoOp = "Query"
	DynamoScan             DynamoOp = "Scan"
	envDynamoPageItems              = "APP_DYNAMODB_PAGE_ITEMS"
	envDynamoPageItemsMax           = "APP_DYNAMODB_PAGE_ITEMS_MAX"
	envDynamoDescribeMax            = "APP_DYNAMODB_DESCRIBE_TABLES_MAX"
	dynamoMaxStartKeyAttrs          = 4
	dynamoDummyCredential           = "AKIDLOCALDUMMY"
	dynamoLocalEndpoint             = "http://127.0.0.1:8000/"
)

// DialectDynamoDB marks a DynamoDB local target. It is never a SQL dialect: only typed requests reach it.
const DialectDynamoDB Dialect = "dynamodb"

// ErrDynamoDenied reports a request the DynamoDB guard refuses.
var ErrDynamoDenied = errors.New("dynamodb request is not allowed")

var (
	dynamoTableRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,255}$`)
	dynamoAttrRE  = regexp.MustCompile(`^[^\x00-\x1f\x7f]{1,255}$`)
	dynamoNumRE   = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][+-]?\d+)?$`)
	dynamoCondOps = map[string]string{"=": "=", "<": "<", "<=": "<=", ">": ">", ">=": ">=", "begins_with": "begins_with"}
)

// DynamoLimits holds the page sizes, read from env like the other viewer limits.
type DynamoLimits struct {
	PageItems    int
	PageItemsMax int
	DescribeMax  int
}

// DynamoLimitsFromEnv reads the page limits, falling back to defaults for unset or invalid values.
func DynamoLimitsFromEnv(lookup func(string) (string, bool)) DynamoLimits {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return DynamoLimits{
		PageItems:    envInt(lookup, envDynamoPageItems, 50),
		PageItemsMax: envInt(lookup, envDynamoPageItemsMax, 500),
		DescribeMax:  envInt(lookup, envDynamoDescribeMax, 25),
	}
}

// DynamoSortCond is an optional condition on the sort key of a Query.
type DynamoSortCond struct {
	Name  string          `json:"name"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

// DynamoRequest is the typed input; no field is ever executed or concatenated into a command.
type DynamoRequest struct {
	Op    DynamoOp
	Table string
	Index string
	// KeyName and KeyValue are the partition key equality of a Query; KeyValue is DynamoDB JSON such as {"S":"x"}.
	KeyName  string
	KeyValue json.RawMessage
	Sort     *DynamoSortCond
	Limit    int
	// StartKey is the previous page's LastEvaluatedKey, DynamoDB JSON.
	StartKey json.RawMessage
	// Confirm must be true for a Scan: it reads every item it passes and is slow on large tables.
	Confirm bool
	// StartTable pages ListTables.
	StartTable string
}

// DynamoPrepared is a request the guard accepted, with its body ready to send.
type DynamoPrepared struct {
	Op          DynamoOp
	Limit       int
	Fingerprint string
	body        []byte
}

func dynamoDeny(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrDynamoDenied, fmt.Sprintf(format, a...))
}

func checkDynamoAttr(raw json.RawMessage) (map[string]string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || len(m) != 1 {
		return nil, dynamoDeny("a key value must be one typed attribute such as {\"S\":\"x\"}")
	}
	out := map[string]string{}
	for t, v := range m {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, dynamoDeny("key value type %q needs a string", t)
		}
		switch t {
		case "S", "B":
		case "N":
			if !dynamoNumRE.MatchString(s) {
				return nil, dynamoDeny("key value is not a number")
			}
		default:
			return nil, dynamoDeny("key value type %q cannot be a key", t)
		}
		out[t] = s
	}
	return out, nil
}

// CheckDynamo validates a request against the allow-list and the limits, then builds the body from re-serialised data.
func CheckDynamo(req DynamoRequest, lim Limits, dl DynamoLimits) (DynamoPrepared, error) {
	pageMax := min(dl.PageItemsMax, lim.MaxRows)
	limit := req.Limit
	if limit <= 0 {
		limit = min(dl.PageItems, pageMax)
	}
	if limit > pageMax {
		limit = pageMax
	}
	body := map[string]any{}
	switch req.Op {
	case DynamoListTables:
		body["Limit"] = min(lim.SchemaRows, 100)
		if req.StartTable != "" {
			if !dynamoTableRE.MatchString(req.StartTable) {
				return DynamoPrepared{}, dynamoDeny("invalid table name")
			}
			body["ExclusiveStartTableName"] = req.StartTable
		}
	case DynamoDescribeTable, DynamoDescribeTTL:
		if !dynamoTableRE.MatchString(req.Table) {
			return DynamoPrepared{}, dynamoDeny("invalid table name")
		}
		body["TableName"] = req.Table
	case DynamoQuery, DynamoScan:
		if !dynamoTableRE.MatchString(req.Table) {
			return DynamoPrepared{}, dynamoDeny("invalid table name")
		}
		body["TableName"] = req.Table
		body["Limit"] = limit
		body["ReturnConsumedCapacity"] = "TOTAL"
		if req.Index != "" {
			if !dynamoTableRE.MatchString(req.Index) {
				return DynamoPrepared{}, dynamoDeny("invalid index name")
			}
			body["IndexName"] = req.Index
		}
		if req.Op == DynamoScan && !req.Confirm {
			return DynamoPrepared{}, dynamoDeny("a scan reads every item it passes: send confirm true to run it")
		}
		if req.Op == DynamoQuery {
			if err := dynamoKeyCondition(body, req); err != nil {
				return DynamoPrepared{}, err
			}
		}
		if len(req.StartKey) > 0 && string(bytes.TrimSpace(req.StartKey)) != "null" {
			sk, err := dynamoStartKey(req.StartKey)
			if err != nil {
				return DynamoPrepared{}, err
			}
			body["ExclusiveStartKey"] = sk
		}
	default:
		return DynamoPrepared{}, dynamoDeny("operation %q is not supported", req.Op)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return DynamoPrepared{}, dynamoDeny("request cannot be encoded")
	}
	if lim.SQLBytes > 0 && len(raw) > lim.SQLBytes {
		return DynamoPrepared{}, dynamoDeny("request is larger than %d bytes", lim.SQLBytes)
	}
	// The fingerprint names the shape only, never key values.
	sum := sha256.Sum256([]byte(string(req.Op) + "|" + req.Table + "|" + req.Index))
	return DynamoPrepared{Op: req.Op, Limit: limit, Fingerprint: hex.EncodeToString(sum[:8]), body: raw}, nil
}

func dynamoKeyCondition(body map[string]any, req DynamoRequest) error {
	if !dynamoAttrRE.MatchString(req.KeyName) {
		return dynamoDeny("a query needs the partition key name")
	}
	pv, err := checkDynamoAttr(req.KeyValue)
	if err != nil {
		return err
	}
	names := map[string]string{"#k": req.KeyName}
	vals := map[string]any{":k": pv}
	cond := "#k = :k"
	if s := req.Sort; s != nil {
		op, ok := dynamoCondOps[s.Op]
		if !ok || !dynamoAttrRE.MatchString(s.Name) {
			return dynamoDeny("invalid sort key condition")
		}
		sv, err := checkDynamoAttr(s.Value)
		if err != nil {
			return err
		}
		names["#s"] = s.Name
		vals[":s"] = sv
		if op == "begins_with" {
			cond += " AND begins_with(#s, :s)"
		} else {
			cond += " AND #s " + op + " :s"
		}
	}
	body["KeyConditionExpression"] = cond
	body["ExpressionAttributeNames"] = names
	body["ExpressionAttributeValues"] = vals
	return nil
}

func dynamoStartKey(raw json.RawMessage) (map[string]any, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || len(m) == 0 || len(m) > dynamoMaxStartKeyAttrs {
		return nil, dynamoDeny("start key must be a key object")
	}
	out := map[string]any{}
	for name, v := range m {
		if !dynamoAttrRE.MatchString(name) {
			return nil, dynamoDeny("invalid start key attribute name")
		}
		typed, err := checkDynamoAttr(v)
		if err != nil {
			return nil, err
		}
		out[name] = typed
	}
	return out, nil
}

// DynamoResult is the engine answer, trimmed to the caps. Items stay DynamoDB JSON.
type DynamoResult struct {
	Tables           []string          `json:"tables,omitempty"`
	LastTable        string            `json:"last_table,omitempty"`
	Table            json.RawMessage   `json:"table,omitempty"`
	TTL              json.RawMessage   `json:"ttl,omitempty"`
	Items            []json.RawMessage `json:"items"`
	Count            int               `json:"count"`
	ScannedCount     int               `json:"scanned_count"`
	LastKey          json.RawMessage   `json:"last_evaluated_key,omitempty"`
	ConsumedCapacity json.RawMessage   `json:"consumed_capacity,omitempty"`
	Truncated        bool              `json:"truncated"`
	DurationMs       int64             `json:"duration_ms"`
}

type dynamoWire struct {
	Type                   string            `json:"__type"`
	Message                string            `json:"message"`
	MessageUpper           string            `json:"Message"`
	TableNames             []string          `json:"TableNames"`
	LastEvaluatedTableName string            `json:"LastEvaluatedTableName"`
	Table                  json.RawMessage   `json:"Table"`
	TTL                    json.RawMessage   `json:"TimeToLiveDescription"`
	Items                  []json.RawMessage `json:"Items"`
	Count                  int               `json:"Count"`
	ScannedCount           int               `json:"ScannedCount"`
	LastKey                json.RawMessage   `json:"LastEvaluatedKey"`
	ConsumedCapacity       json.RawMessage   `json:"ConsumedCapacity"`
}

// dynamoCmd posts the body from stdin with fixed headers; only the allow-listed action name is interpolated.
func dynamoCmd(op DynamoOp, timeout time.Duration) []string {
	secs := max(1, int((timeout+time.Second-1)/time.Second))
	auth := "AWS4-HMAC-SHA256 Credential=" + dynamoDummyCredential + "/20240101/us-east-1/dynamodb/aws4_request, SignedHeaders=host, Signature=0000000000000000000000000000000000000000000000000000000000000000"
	script := fmt.Sprintf(`exec timeout -k 2 %d curl -sS -m %d -X POST -H 'Content-Type: application/x-amz-json-1.0' -H 'X-Amz-Target: DynamoDB_20120810.%s' -H 'Authorization: %s' --data-binary @- %s`,
		secs, secs, string(op), auth, dynamoLocalEndpoint)
	return []string{"sh", "-c", script}
}

// RunDynamo executes a guard-approved request inside the container and parses the answer.
func (t Target) RunDynamo(ctx context.Context, q DynamoPrepared) (DynamoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, t.Limits.Timeout+timeoutMargin)
	defer cancel()
	start := time.Now()
	rc, err := t.Exec.ExecWithInput(ctx, t.ContainerID, dynamoCmd(q.Op, t.Limits.Timeout), bytes.NewReader(q.body))
	if err != nil {
		return DynamoResult{}, wrapExecError(ctx, err)
	}
	defer func() { _ = rc.Close() }()
	capped := &capReader{r: rc, left: t.Limits.MaxBytes}
	raw, err := io.ReadAll(capped)
	if err != nil && !capped.tripd {
		return DynamoResult{}, wrapExecError(ctx, err)
	}
	res, perr := parseDynamo(raw, t.Limits.MaxCellBytes, t.Limits.MaxRows)
	if capped.tripd {
		return DynamoResult{}, errors.New("dbviewer: dynamodb answer is larger than the result byte cap, lower the page size")
	}
	res.DurationMs = time.Since(start).Milliseconds()
	return res, perr
}

// parseDynamo reads one engine answer. Items over the cell cap are replaced by a marker.
func parseDynamo(raw []byte, cellCap, maxItems int) (DynamoResult, error) {
	var w dynamoWire
	if err := json.Unmarshal(bytes.TrimSpace(raw), &w); err != nil {
		return DynamoResult{}, errors.New("dbviewer: dynamodb returned an answer that is not JSON")
	}
	if w.Type != "" {
		msg := w.Message
		if msg == "" {
			msg = w.MessageUpper
		}
		kind := w.Type
		if i := strings.LastIndex(kind, "#"); i >= 0 {
			kind = kind[i+1:]
		}
		return DynamoResult{}, &QueryError{Message: kind + ": " + msg}
	}
	res := DynamoResult{
		Tables: w.TableNames, LastTable: w.LastEvaluatedTableName, Table: w.Table, TTL: w.TTL,
		Items: []json.RawMessage{}, Count: w.Count, ScannedCount: w.ScannedCount,
		LastKey: w.LastKey, ConsumedCapacity: w.ConsumedCapacity,
	}
	for _, it := range w.Items {
		if len(res.Items) >= maxItems {
			res.Truncated = true
			break
		}
		if cellCap > 0 && len(it) > cellCap {
			it = json.RawMessage(`{"__truncated":{"BOOL":true},"__bytes":{"N":"` + strconv.Itoa(len(it)) + `"}}`)
		}
		res.Items = append(res.Items, it)
	}
	return res, nil
}
