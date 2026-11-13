// Package importer imports domains into the asset inventory from a CSV
// file, such as an old domain spreadsheet. Each row's domain is read from
// one column; every other column becomes a field of the item's details.
// Rows naming the same domain are merged, and a domain that is already in
// the inventory (from a provider or entered by hand) gets the imported
// fields merged into its details instead of a second item. Build makes a
// plan without changing anything; Apply carries one out. See
// docs/inventory.md.
package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"golang.org/x/net/publicsuffix"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/inventory"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// MaxRows caps how many data rows one import may have.
const MaxRows = 5000

// NoteKey is the details key the rest of the domain cell is kept under
// ("example.fi (ei laskutusta)" -> "(ei laskutusta)").
const NoteKey = "domain_note"

// Options says how to read the CSV.
type Options struct {
	// DomainColumn is the header of the column holding the domain; the first
	// column when empty.
	DomainColumn string `json:"domain_column"`
	// Columns maps a column header to the details key its values are stored
	// under. An empty key skips the column; a column that isn't listed is
	// stored under its header.
	Columns map[string]string `json:"columns"`
	// Overwrite replaces existing details values that differ from the
	// imported ones; by default those are only reported as conflicts.
	Overwrite bool `json:"overwrite"`
}

// Action is what an import does with one domain.
type Action string

const (
	ActionCreate    Action = "create"
	ActionUpdate    Action = "update"
	ActionUnchanged Action = "unchanged"
	ActionSkip      Action = "skip"
)

// Conflict is a details value the import would change but doesn't: the
// item already has another value (InFile false), or an earlier row of the
// file gave another one (InFile true, Existing is the earlier row's).
type Conflict struct {
	Key      string `json:"key"`
	Existing string `json:"existing"`
	Imported string `json:"imported"`
	InFile   bool   `json:"in_file,omitempty"`
}

// Row is the plan for one domain (or one skipped row).
type Row struct {
	// Lines are the CSV line numbers of the rows merged into this one.
	Lines []int `json:"lines"`
	// Raw is the domain cell as written (of the first row).
	Raw    string `json:"raw"`
	Domain string `json:"domain,omitempty"`
	Action Action `json:"action"`
	// Reason says why a row is skipped.
	Reason string `json:"reason,omitempty"`
	// ItemID is the existing item merged into.
	ItemID string `json:"item_id,omitempty"`
	// Details are the fields read from the file.
	Details map[string]any `json:"details,omitempty"`
	// Changes are the details keys the import sets.
	Changes   []string   `json:"changes,omitempty"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
}

// Plan is what an import of one file does.
type Plan struct {
	Headers      []string `json:"headers"`
	DomainColumn string   `json:"domain_column"`
	Rows         []Row    `json:"rows"`
	Summary      Summary  `json:"summary"`
}

// Summary counts the plan's rows by action.
type Summary struct {
	Create    int `json:"create"`
	Update    int `json:"update"`
	Unchanged int `json:"unchanged"`
	Skip      int `json:"skip"`
	Conflicts int `json:"conflicts"`
}

// ErrInput is wrapped by errors about the file or options themselves.
var ErrInput = errors.New("invalid import")

// Build reads the CSV and plans the import against the inventory as it is
// now. It changes nothing.
func Build(csvText string, opts Options) (*Plan, error) {
	headers, records, err := readCSV(csvText)
	if err != nil {
		return nil, err
	}
	domainCol := 0
	if opts.DomainColumn != "" {
		domainCol = -1
		for i, h := range headers {
			if h == strings.TrimSpace(opts.DomainColumn) {
				domainCol = i
			}
		}
		if domainCol < 0 {
			return nil, fmt.Errorf("%w: no column %q", ErrInput, opts.DomainColumn)
		}
	}
	plan := &Plan{Headers: headers, DomainColumn: headers[domainCol]}

	byDomain := map[string]*Row{}
	var order []string
	for _, rec := range records {
		cell := field(rec.fields, domainCol)
		domain, note, ok := ParseDomainCell(cell)
		if !ok {
			reason := "no domain in the cell"
			if strings.TrimSpace(cell) == "" {
				reason = "the domain cell is empty"
			}
			plan.Rows = append(plan.Rows, Row{Lines: []int{rec.line}, Raw: cell, Action: ActionSkip, Reason: reason})
			continue
		}
		details := map[string]any{}
		if note != "" {
			details[NoteKey] = note
		}
		for i, h := range headers {
			if i == domainCol {
				continue
			}
			key, listed := opts.Columns[h]
			if !listed {
				key = h
			}
			key = strings.TrimSpace(key)
			value := strings.TrimSpace(field(rec.fields, i))
			if key == "" || value == "" {
				continue
			}
			details[key] = value
		}

		row, seen := byDomain[domain]
		if !seen {
			row = &Row{Raw: cell, Domain: domain, Details: map[string]any{}}
			byDomain[domain] = row
			order = append(order, domain)
		}
		row.Lines = append(row.Lines, rec.line)
		for _, key := range sortedKeys(details) {
			value := details[key]
			cur, has := row.Details[key]
			switch {
			case !has:
				row.Details[key] = value
			case cur != value:
				row.Conflicts = append(row.Conflicts, Conflict{Key: key, Existing: fmt.Sprint(cur), Imported: fmt.Sprint(value), InFile: true})
			}
		}
	}

	for _, domain := range order {
		row := byDomain[domain]
		if err := planRow(row, opts.Overwrite); err != nil {
			return nil, err
		}
		plan.Rows = append(plan.Rows, *row)
	}
	sort.SliceStable(plan.Rows, func(i, j int) bool { return plan.Rows[i].Lines[0] < plan.Rows[j].Lines[0] })
	for _, r := range plan.Rows {
		switch r.Action {
		case ActionCreate:
			plan.Summary.Create++
		case ActionUpdate:
			plan.Summary.Update++
		case ActionUnchanged:
			plan.Summary.Unchanged++
		case ActionSkip:
			plan.Summary.Skip++
		}
		plan.Summary.Conflicts += len(r.Conflicts)
	}
	return plan, nil
}

// planRow decides what to do with one domain: create it, or merge its
// fields into the existing item (see existingItem).
func planRow(row *Row, overwrite bool) error {
	item, err := existingItem(row.Domain)
	if err != nil {
		return err
	}
	if item == nil {
		row.Action = ActionCreate
		row.Changes = sortedKeys(row.Details)
		return nil
	}
	row.ItemID = item.ID
	for _, key := range sortedKeys(row.Details) {
		imported := fmt.Sprint(row.Details[key])
		cur, has := item.Details[key]
		current := ""
		if has && cur != nil {
			current = fmt.Sprint(cur)
		}
		switch {
		case strings.TrimSpace(current) == "":
			row.Changes = append(row.Changes, key)
		case current == imported:
		case overwrite:
			row.Changes = append(row.Changes, key)
		default:
			row.Conflicts = append(row.Conflicts, Conflict{Key: key, Existing: current, Imported: imported})
		}
	}
	row.Action = ActionUnchanged
	if len(row.Changes) > 0 {
		row.Action = ActionUpdate
	}
	return nil
}

// existingItem is the domain item an imported domain merges into: of the
// items with its name, an active one before a removed one, then one with
// provider links, then the oldest.
func existingItem(domain string) (*models.InventoryItem, error) {
	items, err := db.FindInventoryItemsByTypeName(models.InventoryItemTypeDomain, domain)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return &inventory.PreferredItems(items)[0], nil
}

// Apply carries out a plan made by Build: it creates the new domain items
// and merges the planned changes into existing ones, as username. It
// returns how many items it created and updated.
func Apply(plan *Plan, username string) (created, updated int, err error) {
	for _, row := range plan.Rows {
		switch row.Action {
		case ActionCreate:
			item := &models.InventoryItem{
				Type:    models.InventoryItemTypeDomain,
				Name:    row.Domain,
				Details: row.Details,
			}
			if err := db.InsertInventoryItem(item, username); err != nil {
				return created, updated, fmt.Errorf("%s: %w", row.Domain, err)
			}
			created++
		case ActionUpdate:
			item, err := db.GetInventoryItem(row.ItemID)
			if err != nil {
				return created, updated, fmt.Errorf("%s: %w", row.Domain, err)
			}
			if item == nil {
				return created, updated, fmt.Errorf("%s: the item was deleted during the import", row.Domain)
			}
			if item.Details == nil {
				item.Details = map[string]any{}
			}
			for _, key := range row.Changes {
				item.Details[key] = row.Details[key]
			}
			if err := db.UpdateInventoryItem(item, username); err != nil {
				return created, updated, fmt.Errorf("%s: %w", row.Domain, err)
			}
			updated++
		}
	}
	return created, updated, nil
}

// ParseDomainCell reads the domain from a cell such as
// "example.fi (ei laskutusta)": the first word, when it is a domain, else
// the first later word that is ("IRTISANOTTU 12/25 example.fi"). The domain
// is returned as its apex in ASCII form. The note is the rest of the cell
// when the domain comes first, else the whole cell. ok is false when no
// word of the cell is a domain.
func ParseDomainCell(cell string) (domain, note string, ok bool) {
	words := strings.Fields(cell)
	for i, w := range words {
		candidate := strings.Trim(w, `()[]{}<>,;:"'!?`)
		apex, err := utils.ApexDomain(candidate)
		if err != nil || !knownTLD(apex) {
			continue
		}
		if i > 0 {
			// The domain is inside a sentence ("HUOM. example.dk ja .ee"):
			// keep the whole cell, which only reads right as a whole.
			return apex, strings.Join(words, " "), true
		}
		return apex, strings.Join(words[1:], " "), true
	}
	return "", "", false
}

// knownTLD reports whether domain ends in a top-level domain on the public
// suffix list, so that "1.2.2024" in a note isn't taken for a domain.
func knownTLD(domain string) bool {
	tld := domain[strings.LastIndex(domain, ".")+1:]
	_, icann := publicsuffix.PublicSuffix(tld)
	return icann
}

type record struct {
	line   int
	fields []string
}

// readCSV parses the file: header row first, comma- or semicolon-separated
// (whichever the header has more of), quoted fields may span lines.
func readCSV(text string) ([]string, []record, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if strings.TrimSpace(text) == "" {
		return nil, nil, fmt.Errorf("%w: the file is empty", ErrInput)
	}
	r := csv.NewReader(strings.NewReader(text))
	firstLine, _, _ := strings.Cut(text, "\n")
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: header row: %v", ErrInput, err)
	}
	headers := make([]string, len(header))
	for i, h := range header {
		headers[i] = strings.TrimSpace(h)
		if headers[i] == "" {
			headers[i] = fmt.Sprintf("Column %d", i+1)
		}
	}
	var records []record
	for {
		fields, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrInput, err)
		}
		line, _ := r.FieldPos(0)
		if len(bytes.TrimSpace([]byte(strings.Join(fields, "")))) == 0 {
			continue
		}
		records = append(records, record{line: line, fields: fields})
		if len(records) > MaxRows {
			return nil, nil, fmt.Errorf("%w: more than %d rows", ErrInput, MaxRows)
		}
	}
	return headers, records, nil
}

func field(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return ""
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
