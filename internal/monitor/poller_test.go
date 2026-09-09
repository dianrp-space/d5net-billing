package monitor

import (
	"testing"
	"time"
)

func TestPollDueHonorsInterval(t *testing.T) {
	now := time.Now()
	if !pollDue(time.Time{}, false, 5*time.Minute, now) {
		t.Fatal("first poll should be due")
	}
	if pollDue(now, true, 5*time.Minute, now.Add(time.Minute)) {
		t.Fatal("should wait for interval")
	}
	if !pollDue(now, true, 5*time.Minute, now.Add(5*time.Minute)) {
		t.Fatal("should poll when interval elapsed")
	}
	if pollDue(now, true, 10*time.Minute, now.Add(5*time.Minute)) {
		t.Fatal("10 min setting should still wait")
	}
}
