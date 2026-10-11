package store

import "testing"

func TestNormalizeJobScheduleCycleInterval(t *testing.T) {
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{}).CycleIntervalSeconds; got != 60 {
		t.Fatalf("zero = %d", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{CycleIntervalSeconds: 30}).CycleIntervalSeconds; got != 60 {
		t.Fatalf("below min = %d", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{CycleIntervalSeconds: 300}).CycleIntervalSeconds; got != 300 {
		t.Fatalf("5 min = %d", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{CycleIntervalSeconds: 90}).CycleIntervalSeconds; got != 60 {
		t.Fatalf("snap down = %d", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{CycleIntervalSeconds: 7200}).CycleIntervalSeconds; got != 3600 {
		t.Fatalf("cap = %d", got)
	}
}

func TestNormalizeIsolirNotifyTime(t *testing.T) {
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{}).IsolirNotifyTime; got != "08:00" {
		t.Fatalf("empty = %q", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{IsolirNotifyTime: "7:30"}).IsolirNotifyTime; got != "07:30" {
		t.Fatalf("7:30 = %q", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{IsolirNotifyTime: "25:00"}).IsolirNotifyTime; got != "08:00" {
		t.Fatalf("invalid = %q", got)
	}
}

func TestNormalizeJobSchedulePollerInterval(t *testing.T) {
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{}).PollerIntervalSeconds; got != 300 {
		t.Fatalf("zero = %d", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{PollerIntervalSeconds: 30}).PollerIntervalSeconds; got != 300 {
		t.Fatalf("below min = %d", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{PollerIntervalSeconds: 600}).PollerIntervalSeconds; got != 600 {
		t.Fatalf("10 min = %d", got)
	}
	if got := NormalizeJobScheduleSettings(JobScheduleSettings{PollerIntervalSeconds: 7200}).PollerIntervalSeconds; got != 3600 {
		t.Fatalf("cap = %d", got)
	}
}
