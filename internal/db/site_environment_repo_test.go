package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestSetAndGetSiteEnvironment(t *testing.T) {
	setupTaskRepoTest(t)

	if err := SetSiteEnvironment(1, "staging", "tester"); err != nil {
		t.Fatalf("failed to set environment: %v", err)
	}

	environments, err := GetAllSiteEnvironments()
	if err != nil {
		t.Fatalf("failed to get environments: %v", err)
	}
	if environments[1] != "staging" {
		t.Fatalf("expected staging, got %q", environments[1])
	}

	if err := SetSiteEnvironment(1, "production", "tester"); err != nil {
		t.Fatalf("failed to update environment: %v", err)
	}
	environments, err = GetAllSiteEnvironments()
	if err != nil {
		t.Fatalf("failed to get environments: %v", err)
	}
	if environments[1] != "production" {
		t.Fatalf("expected production after update, got %q", environments[1])
	}
}

func TestClearSiteEnvironment(t *testing.T) {
	setupTaskRepoTest(t)

	if err := SetSiteEnvironment(2, "development", "tester"); err != nil {
		t.Fatalf("failed to set environment: %v", err)
	}
	if err := ClearSiteEnvironment(2); err != nil {
		t.Fatalf("failed to clear environment: %v", err)
	}

	environments, err := GetAllSiteEnvironments()
	if err != nil {
		t.Fatalf("failed to get environments: %v", err)
	}
	if _, ok := environments[2]; ok {
		t.Fatalf("expected site 2 to be unclassified after clearing")
	}
}

func TestAutoClassifySiteEnvironments(t *testing.T) {
	setupTaskRepoTest(t)

	if err := SetSiteEnvironment(1, "production", "someone"); err != nil {
		t.Fatalf("failed to pre-set environment: %v", err)
	}

	sites := []models.Site{
		{ID: 1, Domain: "www.staging.example.com"}, // already classified, must not change
		{ID: 2, Domain: "www.staging.example.com"},
		{ID: 3, Domain: "app.dev.example.com"},
		{ID: 4, Domain: "app.develop.example.com"},
		{ID: 5, Domain: "app.development.example.com"},
		{ID: 6, Domain: "www.example.com"}, // no match, stays unclassified
	}

	classified, err := AutoClassifySiteEnvironments(sites)
	if err != nil {
		t.Fatalf("AutoClassifySiteEnvironments returned error: %v", err)
	}
	if classified != 4 {
		t.Fatalf("expected 4 sites classified, got %d", classified)
	}

	environments, err := GetAllSiteEnvironments()
	if err != nil {
		t.Fatalf("failed to get environments: %v", err)
	}

	if environments[1] != "production" {
		t.Fatalf("expected site 1 to remain production, got %q", environments[1])
	}
	if environments[2] != "staging" {
		t.Fatalf("expected site 2 to be staging, got %q", environments[2])
	}
	if environments[3] != "development" {
		t.Fatalf("expected site 3 to be development, got %q", environments[3])
	}
	if environments[4] != "development" {
		t.Fatalf("expected site 4 to be development, got %q", environments[4])
	}
	if environments[5] != "development" {
		t.Fatalf("expected site 5 to be development, got %q", environments[5])
	}
	if _, ok := environments[6]; ok {
		t.Fatalf("expected site 6 to remain unclassified")
	}
}
