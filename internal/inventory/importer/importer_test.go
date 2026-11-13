package importer

import (
	"errors"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := config.RunData
	config.RunData.DataDir = dir
	config.RunData.CacheDir = dir
	if err := db.InitInventory(); err != nil {
		t.Fatal(err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		config.RunData = old
	})
}

func TestParseDomainCell(t *testing.T) {
	cases := []struct{ cell, domain, note string }{
		{"example.fi (ei laskutusta)", "example.fi", "(ei laskutusta)"},
		{" example.fi ", "example.fi", ""},
		{"example.fi.", "example.fi", ""},
		{"Example.COM", "example.com", ""},
		{"www.example.fi", "example.fi", ""},
		{"billnäs.fi", "xn--billns-fua.fi", ""},
		{"example.fi OBS laskutus muualle!!", "example.fi", "OBS laskutus muualle!!"},
		{"IRTISANOTTU 12/25 example.fi", "example.fi", "IRTISANOTTU 12/25 example.fi"},
		{"HUOM. example.dk ja .ee", "example.dk", "HUOM. example.dk ja .ee"},
		{"(example.fi)", "example.fi", ""},
	}
	for _, c := range cases {
		domain, note, ok := ParseDomainCell(c.cell)
		if !ok || domain != c.domain || note != c.note {
			t.Errorf("ParseDomainCell(%q) = %q, %q, %v; want %q, %q", c.cell, domain, note, ok, c.domain, c.note)
		}
	}
	for _, cell := range []string{"", "petrigraeffe", "uplink data (ei laskutusta)", "HUOM! 1.2.2024 alk", "12/25"} {
		if d, _, ok := ParseDomainCell(cell); ok {
			t.Errorf("ParseDomainCell(%q) found %q", cell, d)
		}
	}
}

const testCSV = "\ufeffDomain,Customer,Registrar,Notes\n" +
	"example.fi (ei laskutusta),Example Oy,Traficom,\n" +
	"example.com,Example Oy,Joker,\"two\nlines\"\n" +
	"example.com,Example Oy,Other,\n" +
	"no domain here,Someone,,\n" +
	"existing.fi,New Customer,Traficom,\n" +
	",,,\n"

func TestBuildAndApply(t *testing.T) {
	setupDB(t)
	existing := &models.InventoryItem{
		Type: models.InventoryItemTypeDomain, Name: "existing.fi",
		Details: map[string]any{"Customer": "Old Customer"},
	}
	if err := db.InsertInventoryItem(existing, "tester"); err != nil {
		t.Fatal(err)
	}

	plan, err := Build(testCSV, Options{Columns: map[string]string{"Registrar": "registrar", "Notes": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.DomainColumn != "Domain" || len(plan.Headers) != 4 {
		t.Fatalf("headers: %+v", plan)
	}
	want := Summary{Create: 2, Update: 1, Skip: 1, Conflicts: 2}
	if plan.Summary != want {
		t.Fatalf("summary = %+v, want %+v", plan.Summary, want)
	}
	byDomain := map[string]Row{}
	for _, r := range plan.Rows {
		byDomain[r.Domain] = r
	}
	fi := byDomain["example.fi"]
	if fi.Details["registrar"] != "Traficom" || fi.Details["Customer"] != "Example Oy" || fi.Details[NoteKey] != "(ei laskutusta)" {
		t.Errorf("example.fi details: %+v", fi.Details)
	}
	if _, ok := fi.Details["Notes"]; ok {
		t.Errorf("skipped column imported: %+v", fi.Details)
	}
	com := byDomain["example.com"]
	if len(com.Lines) != 2 || com.Lines[0] != 3 || com.Lines[1] != 5 || com.Details["registrar"] != "Joker" ||
		len(com.Conflicts) != 1 || !com.Conflicts[0].InFile || com.Conflicts[0].Imported != "Other" {
		t.Errorf("merged rows: %+v", com)
	}
	ex := byDomain["existing.fi"]
	if ex.Action != ActionUpdate || ex.ItemID != existing.ID || len(ex.Changes) != 1 || ex.Changes[0] != "registrar" ||
		len(ex.Conflicts) != 1 || ex.Conflicts[0].Existing != "Old Customer" {
		t.Errorf("existing.fi: %+v", ex)
	}

	// Build changes nothing.
	if items, _ := db.ListInventoryItems(db.InventoryItemFilter{}); len(items) != 1 {
		t.Fatalf("dry run wrote items: %d", len(items))
	}

	created, updated, err := Apply(plan, "importer")
	if err != nil || created != 2 || updated != 1 {
		t.Fatalf("apply: %d %d %v", created, updated, err)
	}
	got, _ := db.GetInventoryItem(existing.ID)
	if got.Details["Customer"] != "Old Customer" || got.Details["registrar"] != "Traficom" || got.UpdatedBy != "importer" {
		t.Errorf("merged item: %+v", got)
	}

	// Importing again changes nothing; with overwrite the conflict is taken.
	plan, _ = Build(testCSV, Options{Columns: map[string]string{"Registrar": "registrar", "Notes": ""}})
	if plan.Summary.Create != 0 || plan.Summary.Update != 0 || plan.Summary.Unchanged != 3 {
		t.Errorf("re-import: %+v", plan.Summary)
	}
	plan, _ = Build(testCSV, Options{Columns: map[string]string{"Registrar": "registrar", "Notes": ""}, Overwrite: true})
	if _, _, err := Apply(plan, "importer"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetInventoryItem(existing.ID); got.Details["Customer"] != "New Customer" {
		t.Errorf("overwrite: %+v", got.Details)
	}
}

func TestBuildMergesIntoPreferredItem(t *testing.T) {
	setupDB(t)
	removed := &models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: "example.fi", Status: models.InventoryItemStatusRemoved}
	active := &models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: "example.fi"}
	for _, it := range []*models.InventoryItem{removed, active} {
		if err := db.InsertInventoryItem(it, "tester"); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := Build("Domain,Customer\nexample.fi,Example Oy\n", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Rows[0].ItemID != active.ID {
		t.Errorf("merged into %s, want the active item %s", plan.Rows[0].ItemID, active.ID)
	}
}

func TestBuildInputErrors(t *testing.T) {
	setupDB(t)
	if _, err := Build("", Options{}); !errors.Is(err, ErrInput) {
		t.Errorf("empty file: %v", err)
	}
	if _, err := Build("Domain\nexample.fi\n", Options{DomainColumn: "Nope"}); !errors.Is(err, ErrInput) {
		t.Errorf("unknown column: %v", err)
	}
	// Semicolon-separated files (Excel with a Finnish locale) work too.
	plan, err := Build("Domain;Customer\nexample.fi;Example Oy\n", Options{})
	if err != nil || plan.Rows[0].Details["Customer"] != "Example Oy" {
		t.Errorf("semicolons: %+v %v", plan, err)
	}
}
