package platformimport

import "testing"

func TestParseDBVersions(t *testing.T) {
	got, err := ParseDBVersions("postgres=16, Redis=7.4")
	if err != nil || got["postgres"] != "16" || got["redis"] != "7.4" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := ParseDBVersions("postgres"); err == nil {
		t.Fatal("want error for a missing version")
	}
}

func TestPlanDatabaseWithoutSourceVersion(t *testing.T) {
	d := &Discovery{Platform: Heroku, Databases: []Database{{SourceID: "a", Name: "db", Engine: "postgres"}, {SourceID: "b", Name: "cache", Engine: "redis"}}}
	p := BuildPlan(d, PlanOptions{DBVersions: map[string]string{"postgres": "16"}, LatestDBVersions: map[string]string{"postgres": "18", "redis": "8.2"}})
	if len(p.Databases) != 2 || p.Databases[0].Version != "16" || p.Databases[1].Version != "8.2" {
		t.Fatalf("databases %+v", p.Databases)
	}
	p = BuildPlan(d, PlanOptions{})
	if len(p.Databases) != 0 {
		t.Fatalf("without a catalog the databases stay unsupported: %+v", p.Databases)
	}
}
