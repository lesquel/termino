package infrastructure_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lesquel/termino/internal/features/share/domain"
	"github.com/lesquel/termino/internal/features/share/infrastructure"
)

var update = flag.Bool("update", false, "rewrite golden files")

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func sampleProject() domain.ProjectFile {
	return domain.ProjectFile{
		Version: domain.FormatVersion, Key: "PROJ", Name: "termino", Methodology: "scrum",
		Columns: []domain.ColumnEntry{{Name: "Backlog", Position: 0}, {Name: "In Progress", Position: 1, WIPLimit: 3}},
		Members: []domain.MemberEntry{{Name: "Ana", Kind: "human"}, {Name: "claude", Kind: "agent"}},
		Labels:  []domain.LabelEntry{{Name: "bug"}, {Name: "ui", Color: "#7aa2f7"}},
		Sprints: []domain.SprintEntry{
			{Name: "Sprint 1", Start: date(2026, 10, 20), End: date(2026, 11, 3), State: "active"},
			{Name: "Sprint 2", State: "draft"},
		},
	}
}

func sampleTask() domain.TaskFile {
	points := 3
	due := date(2026, 10, 30)
	return domain.TaskFile{
		Version: domain.FormatVersion, ID: "3f6c2a9e-8d1b-4c57-9a0e-2b7d4e1f6a33", Number: 3,
		Title: `Probar los "dialogos" nuevos`, Column: "Backlog", Position: 2.5, Priority: "medium",
		Points: &points, Assignee: "Ana", Labels: []string{"bug", "ui"}, Sprint: "Sprint 1", Due: &due,
		CreatedAt:   time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 10, 17, 18, 10, 0, 0, time.UTC),
		UpdatedBy:   "Ana",
		Description: "Multi-line text\nkept as written.\nQuotes \"\"\" and a backslash \\ stay.",
	}
}

func minimalTask() domain.TaskFile {
	at := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	return domain.TaskFile{
		Version: domain.FormatVersion, ID: "0b1c", Number: 1, Title: "Minimal", Column: "Done",
		Position: 1, Priority: "low", CreatedAt: at, UpdatedAt: at, Description: "one line\twith a tab",
	}
}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestEncodeProject_Golden(t *testing.T) {
	checkGolden(t, "project.golden.toml", infrastructure.TOMLCodec{}.EncodeProject(sampleProject()))
}

func TestEncodeTask_Golden(t *testing.T) {
	c := infrastructure.TOMLCodec{}
	checkGolden(t, "task_full.golden.toml", c.EncodeTask(sampleTask()))
	checkGolden(t, "task_minimal.golden.toml", c.EncodeTask(minimalTask()))
}

func TestEncodeTask_Deterministic(t *testing.T) {
	c := infrastructure.TOMLCodec{}
	first := c.EncodeTask(sampleTask())
	for i := 0; i < 5; i++ {
		if string(c.EncodeTask(sampleTask())) != string(first) {
			t.Fatal("encoding is not deterministic")
		}
	}
}

func TestEncodeTask_ConvertsTimesToUTC(t *testing.T) {
	task := minimalTask()
	task.CreatedAt = time.Date(2026, 10, 17, 15, 0, 0, 0, time.FixedZone("x", -3*3600))
	got, err := infrastructure.TOMLCodec{}.DecodeTask(infrastructure.TOMLCodec{}.EncodeTask(task))
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt.Location() != time.UTC || got.CreatedAt.Hour() != 18 {
		t.Errorf("CreatedAt = %v, want 18:00 UTC", got.CreatedAt)
	}
}

func TestTaskRoundTrip(t *testing.T) {
	c := infrastructure.TOMLCodec{}
	descriptions := []string{
		"", "plain", "multi\nline", "ends with newline\n", "\nstarts with newline",
		`ends with quote"`, "ends with backslash \\", "crlf\r\nline", "ctrl \x01 char",
		"a \"\"\"\"\" run\nof quotes", "trailing ws  \n  ",
	}
	for _, d := range descriptions {
		for _, task := range []domain.TaskFile{sampleTask(), minimalTask()} {
			task.Description = d
			got, err := c.DecodeTask(c.EncodeTask(task))
			if err != nil {
				t.Fatalf("description %q: %v\n%s", d, err, c.EncodeTask(task))
			}
			if !reflect.DeepEqual(got, task) {
				t.Errorf("round trip mismatch for %q\n got: %+v\nwant: %+v", d, got, task)
			}
		}
	}
}

func TestProjectRoundTrip(t *testing.T) {
	c := infrastructure.TOMLCodec{}
	want := sampleProject()
	got, err := c.DecodeProject(c.EncodeProject(want))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestDecodeTask_SpecExample(t *testing.T) {
	src := `version = 1
id = "3f6c2a9e"
number = 3
title = "T"
column = "Backlog"
position = 2
priority = "medium"
created_at = 2026-10-17T18:00:00Z
updated_at = 2026-10-17T18:10:00Z
some_future_key = "ignored"
description = """
Multi-line text
kept as written.
"""
`
	got, err := infrastructure.TOMLCodec{}.DecodeTask([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "Multi-line text\nkept as written." || got.Position != 2 {
		t.Errorf("decoded %+v", got)
	}
}

func TestDecode_Errors(t *testing.T) {
	c := infrastructure.TOMLCodec{}
	valid := string(c.EncodeTask(minimalTask()))
	cases := []struct {
		name string
		src  string
		want error
	}{
		{"conflict markers", "<<<<<<< HEAD\n" + valid + "=======\n" + valid + ">>>>>>> other\n", domain.ErrConflictMarkers},
		{"invalid toml", "version = = 1\n", domain.ErrInvalidFile},
		{"missing id", "version = 1\ntitle = \"x\"\n", domain.ErrInvalidFile},
		{"missing version", "id = \"a\"\ntitle = \"x\"\n", domain.ErrInvalidFile},
		{"newer version", "version = 2\nid = \"a\"\ntitle = \"x\"\n", domain.ErrUnsupportedVersion},
		{"wrong type", "version = 1\nid = \"a\"\ntitle = \"x\"\nnumber = \"three\"\n", domain.ErrInvalidFile},
	}
	for _, tc := range cases {
		if _, err := c.DecodeTask([]byte(tc.src)); !errors.Is(err, tc.want) {
			t.Errorf("%s: DecodeTask err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := c.DecodeProject([]byte("version = 3\nkey = \"P\"\n")); !errors.Is(err, domain.ErrUnsupportedVersion) {
		t.Errorf("DecodeProject newer version err = %v", err)
	}
	if _, err := c.DecodeProject([]byte("version = 1\nname = \"no key\"\n")); !errors.Is(err, domain.ErrInvalidFile) {
		t.Errorf("DecodeProject missing key err = %v", err)
	}
	if _, err := c.DecodeProject([]byte(">>>>>>> theirs\n")); !errors.Is(err, domain.ErrConflictMarkers) {
		t.Errorf("DecodeProject conflict err = %v", err)
	}
}
