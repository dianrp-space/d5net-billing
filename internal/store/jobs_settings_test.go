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
