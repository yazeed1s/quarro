package jobs

import (
	"testing"
	"time"
)

func TestCancelStopsContext(t *testing.T) {
	r := NewRegistry()
	ctx := r.Start("job-1", 0)
	if !r.Cancel("job-1") {
		t.Fatal("Cancel() = false, want true for a running job")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context was not cancelled")
	}
}

func TestCancelUnknownJobReturnsFalse(t *testing.T) {
	r := NewRegistry()
	if r.Cancel("does-not-exist") {
		t.Fatal("Cancel() = true for an unknown job")
	}
}

func TestDoneReleasesJob(t *testing.T) {
	r := NewRegistry()
	r.Start("job-1", 0)
	r.Done("job-1")
	if r.Cancel("job-1") {
		t.Fatal(
			"Cancel() = true after Done(), job should be released",
		)
	}
}

func TestTimeoutCancelsContext(t *testing.T) {
	r := NewRegistry()
	ctx := r.Start("job-1", 20*time.Millisecond)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context did not time out")
	}
	if ctx.Err() != nil && ctx.Err().Error() == "" {
		t.Fatal("expected a deadline exceeded error")
	}
}

func TestIndependentJobs(t *testing.T) {
	r := NewRegistry()
	ctx1 := r.Start("job-1", 0)
	r.Start("job-2", 0)
	r.Cancel("job-1")
	select {
	case <-ctx1.Done():
	default:
		t.Fatal("job-1 should be cancelled")
	}
	if !r.Cancel("job-2") {
		t.Fatal("job-2 should still be running and cancellable")
	}
}
