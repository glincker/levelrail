package main

import (
	"strings"
	"testing"
)

func TestRunDatabasesResourceRecommendation_JSON(t *testing.T) {
	testResourceRecommendationJSON(t, "databases", "main")
}

func TestRunDatabasesResourceRecommendation_Human(t *testing.T) {
	stdout := testResourceRecommendationHuman(t, "databases", "main", "database")
	if !strings.Contains(stdout, "database:") {
		t.Errorf("stdout = %q, want the database label", stdout)
	}
}

func TestRunDatabasesResourceRecommendation_MissingName(t *testing.T) {
	testResourceRecommendationMissingName(t, "databases")
}
