package cloudspend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

type fakeAWS struct {
	calls map[string]int
	resp  map[string]string
	err   map[string]error
}

func (f *fakeAWS) call(_ context.Context, _, target string, _, out any) error {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[target]++
	if e := f.err[target]; e != nil {
		return e
	}
	return json.Unmarshal([]byte(f.resp[target]), out)
}

func awsCfg(ce bool) func() Config {
	return func() Config {
		return Config{AWSCostExplorer: ce, MaxPages: 3, Now: func() time.Time { return time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC) }}
	}
}

const ceTagged = `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-11-09","End":"2026-11-10"},"Estimated":true,
 "Groups":[{"Keys":["levelrail:project$web","Amazon EC2"],"Metrics":{"UnblendedCost":{"Amount":"2.5","Unit":"USD"}}},
 {"Keys":["levelrail:project$","Amazon S3"],"Metrics":{"UnblendedCost":{"Amount":"0.5","Unit":"USD"}}}],"Unknown":1}]}`

func TestAWSDisabledMakesNoCall(t *testing.T) {
	f := &fakeAWS{}
	rep, err := newAWSReader(f, "123", awsCfg(false)).Spend(context.Background(), SpendQuery{AccountID: "a"})
	if err != nil || rep.State != StateDisabled || rep.FixHint == "" || len(f.calls) != 0 {
		t.Fatalf("rep %+v err %v calls %v", rep, err, f.calls)
	}
}

func TestAWSSpendGroupsAndMemoizesPerDay(t *testing.T) {
	f := &fakeAWS{resp: map[string]string{awsCETarget + "GetCostAndUsage": ceTagged}}
	r := newAWSReader(f, "123", awsCfg(true))
	rep, err := r.Spend(context.Background(), SpendQuery{AccountID: "a"})
	if err != nil || rep.State != StateOK || !rep.Estimated {
		t.Fatalf("rep %+v err %v", rep, err)
	}
	if len(rep.ByProject) != 1 || rep.ByProject[0].Name != "web" || len(rep.ByService) != 2 || rep.Totals[0].Amount != 3 {
		t.Fatalf("grouping wrong: %+v", rep)
	}
	if _, err := r.Spend(context.Background(), SpendQuery{AccountID: "a"}); err != nil {
		t.Fatal(err)
	}
	if n := f.calls[awsCETarget+"GetCostAndUsage"]; n != 1 {
		t.Fatalf("Cost Explorer calls = %d, want 1 per day", n)
	}
}

func TestAWSTagNotActive(t *testing.T) {
	untagged := strings.ReplaceAll(ceTagged, "levelrail:project$web", "levelrail:project$")
	f := &fakeAWS{resp: map[string]string{awsCETarget + "GetCostAndUsage": untagged}}
	rep, _ := newAWSReader(f, "123", awsCfg(true)).Spend(context.Background(), SpendQuery{AccountID: "a"})
	if rep.State != StateTagNotActive || rep.FixHint == "" || len(rep.ByProject) != 0 || len(rep.ByService) != 2 {
		t.Fatalf("rep %+v", rep)
	}
	f = &fakeAWS{err: map[string]error{awsCETarget + "GetCostAndUsage": &ProviderError{Status: 400, Code: "DataUnavailableException"}}}
	rep, err := newAWSReader(f, "123", awsCfg(true)).Spend(context.Background(), SpendQuery{AccountID: "a"})
	if err != nil || rep.State != stateDataUnavailable || len(rep.Totals) != 0 {
		t.Fatalf("rep %+v err %v", rep, err)
	}
}

func TestAWSForecastAddsProviderRemainder(t *testing.T) {
	f := &fakeAWS{resp: map[string]string{
		awsCETarget + "GetCostAndUsage": ceTagged,
		awsCETarget + "GetCostForecast": `{"Total":{"Amount":"27","Unit":"USD"}}`,
	}}
	r := newAWSReader(f, "123", awsCfg(true))
	fc, err := r.Forecast(context.Background(), SpendQuery{AccountID: "a"})
	if err != nil || fc.Amount != 30 || fc.Source != ForecastProvider {
		t.Fatalf("fc %+v err %v", fc, err)
	}
	f.err = map[string]error{awsCETarget + "GetCostForecast": &ProviderError{Status: 400}}
	r = newAWSReader(f, "123", awsCfg(true))
	if fc, _ = r.Forecast(context.Background(), SpendQuery{AccountID: "a"}); fc.Source != ForecastLinear {
		t.Fatalf("want linear fallback, got %+v", fc)
	}
}

func TestAWSBudgetsAndPrice(t *testing.T) {
	f := &fakeAWS{resp: map[string]string{
		awsBudgetTarget: `{"Budgets":[{"BudgetName":"m","BudgetLimit":{"Amount":"100","Unit":"USD"},"TimeUnit":"MONTHLY",
		 "CalculatedSpend":{"ActualSpend":{"Amount":"40","Unit":"USD"},"ForecastedSpend":{"Amount":"90","Unit":"USD"}},"LastUpdatedTime":1.7e9}]}`,
		awsPriceTarget: `{"PriceList":["{\"terms\":{\"OnDemand\":{\"x\":{\"priceDimensions\":{\"y\":{\"unit\":\"Hrs\",\"pricePerUnit\":{\"USD\":\"0.0104\"}}}}}}}"]}`,
	}}
	r := newAWSReader(f, "123", awsCfg(false))
	b, err := r.Budgets(context.Background(), SpendQuery{})
	if err != nil || len(b) != 1 || b[0].ActualSpend != 40 || b[0].Currency != "USD" || b[0].DataAsOf == nil {
		t.Fatalf("budgets %+v err %v", b, err)
	}
	p, err := r.EstimatePrice(context.Background(), PriceQuery{ServiceCode: "AmazonEC2", Region: "eu-west-1", HoursPerMonth: 730})
	if err != nil || !p.Estimated || p.Monthly < 7.5 || p.Monthly > 7.6 {
		t.Fatalf("price %+v err %v", p, err)
	}
}

func TestAWSHTTPSignsRequest(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	a := &awsHTTP{creds: aws.Credentials{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "x"}, client: srv.Client(), signer: v4.NewSigner(), base: srv.URL, now: time.Now}
	var out map[string]any
	if err := a.call(context.Background(), "ce", awsCETarget+"GetCostAndUsage", map[string]string{}, &out); err != nil {
		t.Fatal(err)
	}
	if got.Header.Get("X-Amz-Target") != awsCETarget+"GetCostAndUsage" || !strings.Contains(got.Header.Get("Authorization"), "/ce/aws4_request") {
		t.Fatalf("headers %v", got.Header)
	}
}
