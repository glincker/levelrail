package dbviewer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func dynTestLimits() (Limits, DynamoLimits) {
	return Limits{Timeout: time.Second, MaxRows: 100, MaxBytes: 1 << 20, MaxCellBytes: 200, SchemaRows: 50, SQLBytes: 4096},
		DynamoLimits{PageItems: 10, PageItemsMax: 20, DescribeMax: 5}
}

func TestCheckDynamoAccepts(t *testing.T) {
	lim, dl := dynTestLimits()
	q, err := CheckDynamo(DynamoRequest{
		Op: DynamoQuery, Table: "orders", KeyName: "pk", KeyValue: json.RawMessage(`{"S":"a"}`),
		Sort: &DynamoSortCond{Name: "sk", Op: "begins_with", Value: json.RawMessage(`{"S":"x"}`)}, Limit: 500,
	}, lim, dl)
	if err != nil {
		t.Fatal(err)
	}
	if q.Limit != 20 {
		t.Errorf("limit = %d, want the page max 20", q.Limit)
	}
	if !strings.Contains(string(q.body), `begins_with(#s, :s)`) || !strings.Contains(string(q.body), `":k":{"S":"a"}`) {
		t.Errorf("body = %s", q.body)
	}
	if _, err := CheckDynamo(DynamoRequest{Op: DynamoScan, Table: "orders", Confirm: true}, lim, dl); err != nil {
		t.Errorf("confirmed scan refused: %v", err)
	}
}

func TestCheckDynamoRejects(t *testing.T) {
	lim, dl := dynTestLimits()
	cases := map[string]DynamoRequest{
		"put":            {Op: "PutItem", Table: "orders"},
		"delete":         {Op: "DeleteItem", Table: "orders"},
		"update":         {Op: "UpdateItem", Table: "orders"},
		"create":         {Op: "CreateTable", Table: "orders"},
		"drop":           {Op: "DeleteTable", Table: "orders"},
		"scan unconfirm": {Op: DynamoScan, Table: "orders"},
		"short table":    {Op: DynamoDescribeTable, Table: "ab"},
		"table inject":   {Op: DynamoDescribeTable, Table: "orders;rm"},
		"no key":         {Op: DynamoQuery, Table: "orders"},
		"bool key":       {Op: DynamoQuery, Table: "orders", KeyName: "pk", KeyValue: json.RawMessage(`{"BOOL":"true"}`)},
		"bad number":     {Op: DynamoQuery, Table: "orders", KeyName: "pk", KeyValue: json.RawMessage(`{"N":"1;2"}`)},
		"bad sort op":    {Op: DynamoQuery, Table: "orders", KeyName: "pk", KeyValue: json.RawMessage(`{"S":"a"}`), Sort: &DynamoSortCond{Name: "s", Op: "OR 1=1", Value: json.RawMessage(`{"S":"a"}`)}},
		"big start key":  {Op: DynamoScan, Table: "orders", Confirm: true, StartKey: json.RawMessage(`{"a":{"S":"1"},"b":{"S":"1"},"c":{"S":"1"},"d":{"S":"1"},"e":{"S":"1"}}`)},
	}
	for name, req := range cases {
		if _, err := CheckDynamo(req, lim, dl); !errors.Is(err, ErrDynamoDenied) {
			t.Errorf("%s: err = %v, want denied", name, err)
		}
	}
}

type dynStubExec struct {
	out   string
	cmd   []string
	input string
}

func (s *dynStubExec) ExecWithInput(_ context.Context, _ string, cmd []string, in io.Reader) (io.ReadCloser, error) {
	s.cmd = cmd
	b, _ := io.ReadAll(in)
	s.input = string(b)
	return io.NopCloser(strings.NewReader(s.out)), nil
}

func TestRunDynamoParsesAndKeepsBodyOffArgv(t *testing.T) {
	lim, dl := dynTestLimits()
	q, err := CheckDynamo(DynamoRequest{Op: DynamoQuery, Table: "orders", KeyName: "pk", KeyValue: json.RawMessage(`{"S":"secretvalue"}`)}, lim, dl)
	if err != nil {
		t.Fatal(err)
	}
	big := `{"x":{"S":"` + strings.Repeat("a", 400) + `"}}`
	st := &dynStubExec{out: `{"Items":[{"pk":{"S":"a"}},` + big + `],"Count":2,"ScannedCount":2,"LastEvaluatedKey":{"pk":{"S":"a"}}}`}
	res, err := Target{Exec: st, Limits: lim}.RunDynamo(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 || !strings.Contains(string(res.Items[1]), "__truncated") || res.LastKey == nil {
		t.Errorf("res = %+v", res)
	}
	if strings.Contains(strings.Join(st.cmd, " "), "secretvalue") || !strings.Contains(st.input, "secretvalue") {
		t.Errorf("key value must travel on stdin only, cmd = %v", st.cmd)
	}
	if !strings.Contains(strings.Join(st.cmd, " "), "DynamoDB_20120810.Query") || !strings.Contains(strings.Join(st.cmd, " "), "127.0.0.1:8000") {
		t.Errorf("cmd = %v", st.cmd)
	}
}

func TestRunDynamoServerError(t *testing.T) {
	lim, dl := dynTestLimits()
	q, _ := CheckDynamo(DynamoRequest{Op: DynamoDescribeTable, Table: "orders"}, lim, dl)
	st := &dynStubExec{out: `{"__type":"com.amazonaws.dynamodb.v20120810#ResourceNotFoundException","message":"Cannot do operations on a non-existent table"}`}
	_, err := Target{Exec: st, Limits: lim}.RunDynamo(context.Background(), q)
	var qe *QueryError
	if !errors.As(err, &qe) || !strings.HasPrefix(qe.Message, "ResourceNotFoundException") {
		t.Fatalf("err = %v", err)
	}
}

func TestDynamoLimitsFromEnv(t *testing.T) {
	t.Setenv(envDynamoPageItems, "7")
	if got := DynamoLimitsFromEnv(os.LookupEnv).PageItems; got != 7 {
		t.Errorf("page items = %d", got)
	}
}
