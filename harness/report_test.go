package harness

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestReport_MarkdownAndCSV(t *testing.T) {
	opts := DefaultOptions()
	opts.Config.N = 60
	opts.Replicas = 2
	results := Run(DefaultMethods(), opts)

	md := Markdown(results, opts)
	for _, m := range DefaultMethods() {
		if !strings.Contains(md, m.Name()) {
			t.Fatalf("markdown is missing method %s", m.Name())
		}
	}
	for _, s := range Scenarios[1:] {
		if !strings.Contains(md, "| "+string(s)+" ") {
			t.Fatalf("markdown is missing scenario %s", s)
		}
	}

	var buf bytes.Buffer
	if err := WriteCSV(&buf, results); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(results)+1 {
		t.Fatalf("got %d CSV rows, want %d", len(rows), len(results)+1)
	}
}
