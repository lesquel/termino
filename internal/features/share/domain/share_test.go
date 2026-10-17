package domain_test

import (
	"testing"
	"time"

	"github.com/lesquel/termino/internal/features/share/domain"
)

func TestLosesNumber(t *testing.T) {
	early := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	late := early.Add(time.Minute)
	a := domain.TaskFile{ID: "aaa", CreatedAt: early}
	b := domain.TaskFile{ID: "bbb", CreatedAt: late}
	if domain.LosesNumber(a, b) || !domain.LosesNumber(b, a) {
		t.Error("the later created task must lose the number")
	}
	b.CreatedAt = early
	if domain.LosesNumber(a, b) || !domain.LosesNumber(b, a) {
		t.Error("on a tie the greater UUID must lose the number")
	}
}

func TestTaskFilePath(t *testing.T) {
	if got := domain.TaskFilePath("abc"); got != "tasks/abc.toml" {
		t.Errorf("TaskFilePath = %q", got)
	}
}
