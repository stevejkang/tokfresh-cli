package schedule_test

import (
	"strings"
	"testing"

	"github.com/stevejkang/tokfresh-cli/internal/schedule"
)

func TestCalculateSchedule(t *testing.T) {
	result := schedule.Calculate("06:00")
	expected := []string{"06:00", "11:00", "16:00", "21:00", "02:00"}
	if len(result) != 5 {
		t.Fatalf("expected 5 slots, got %d", len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("slot %d: got %s, want %s", i, result[i], v)
		}
	}
}

func TestCalculateScheduleWrap(t *testing.T) {
	result := schedule.Calculate("22:00")
	expected := []string{"22:00", "03:00", "08:00", "13:00", "18:00"}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("slot %d: got %s, want %s", i, result[i], v)
		}
	}
}

func TestCalculateScheduleWithMinutes(t *testing.T) {
	result := schedule.Calculate("06:30")
	expected := []string{"06:30", "11:30", "16:30", "21:30", "02:30"}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("slot %d: got %s, want %s", i, result[i], v)
		}
	}
}

func TestGetResetTime(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"06:00", "11:00"},
		{"21:00", "02:00"},
		{"22:00", "03:00"},
		{"00:00", "05:00"},
		{"19:30", "00:30"},
	}
	for _, tt := range tests {
		got := schedule.GetResetTime(tt.input)
		if got != tt.want {
			t.Errorf("GetResetTime(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestToCronFormat(t *testing.T) {
	times := []string{"06:00", "11:00", "16:00", "21:00", "02:00"}
	cron := schedule.ToCron(times, "UTC")

	if !strings.Contains(cron, "* * *") {
		t.Errorf("cron missing '* * *' suffix: %s", cron)
	}

	parts := strings.Fields(cron)
	if len(parts) != 5 {
		t.Fatalf("expected 5 cron fields, got %d: %s", len(parts), cron)
	}

	hours := strings.Split(parts[1], ",")
	if len(hours) != 4 {
		t.Errorf("expected 4 hours in cron, got %d: %s", len(hours), parts[1])
	}

	if parts[0] != "0" {
		t.Errorf("expected minute 0 for UTC, got %s", parts[0])
	}
	expectedHours := "6,11,16,21"
	if parts[1] != expectedHours {
		t.Errorf("expected hours %s, got %s", expectedHours, parts[1])
	}
}

func TestToCronOnlyActiveSlots(t *testing.T) {
	times := []string{"06:00", "11:00", "16:00", "21:00", "02:00"}
	cron := schedule.ToCron(times, "UTC")
	parts := strings.Fields(cron)
	hours := strings.Split(parts[1], ",")
	if len(hours) != 4 {
		t.Errorf("expected 4 active triggers, got %d", len(hours))
	}
	for _, h := range hours {
		if h == "2" {
			t.Error("02:00 (5th slot) should not be in active cron triggers")
		}
	}
}

func TestGetNextTrigger(t *testing.T) {
	sched := []string{"06:00", "11:00", "16:00", "21:00", "02:00"}
	triggerTime, label := schedule.GetNextTrigger(sched, "UTC")

	if triggerTime.IsZero() {
		t.Error("expected non-zero trigger time")
	}
	if label == "" {
		t.Error("expected non-empty label")
	}
	if !strings.Contains(label, "Today") && !strings.Contains(label, "Tomorrow") {
		t.Errorf("label should contain Today or Tomorrow, got: %s", label)
	}
}

func TestDetectTimezone(t *testing.T) {
	tz := schedule.DetectTimezone()
	if tz == "" {
		t.Error("DetectTimezone returned empty string")
	}
}

func TestActiveTriggerCount(t *testing.T) {
	if schedule.ActiveTriggerCount != 4 {
		t.Errorf("ActiveTriggerCount = %d, want 4", schedule.ActiveTriggerCount)
	}
}
