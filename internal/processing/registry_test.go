package processing

import "testing"

func TestRegistryReserveRefusesItemThatIsBusy(t *testing.T) {
	r := newJobRegistry()

	if _, ok := r.reserve("a"); !ok {
		t.Fatal("reserve refused an idle item")
	}
	if _, ok := r.reserve("a"); ok {
		t.Error("reserve accepted a second job for an item that is already busy")
	}
	if _, ok := r.reserve("b"); !ok {
		t.Error("reserve refused an unrelated item")
	}
}

func TestRegistryCancelDropsJobThatHasNotStarted(t *testing.T) {
	r := newJobRegistry()

	queuedToken, _ := r.reserve("a")

	if wait := r.cancel("a"); wait != nil {
		t.Error("cancelling a queued job returned a channel; there is nothing running to wait for")
	}
	if _, ok := r.reserve("a"); !ok {
		t.Fatal("cancelling a queued job did not free the item for a replacement")
	}

	freshToken, _ := r.reserve("b") // unrelated item, still reserved
	if r.claim("a", queuedToken, func() {}) {
		t.Error("claim accepted a job whose reservation was cancelled while it was queued")
	}
	if !r.claim("b", freshToken, func() {}) {
		t.Error("claim rejected the job that owns the reservation")
	}
	if r.claim("c", 1, func() {}) {
		t.Error("claim accepted a job that was never reserved")
	}
}

func TestRegistryCancelWaitsForRunningJob(t *testing.T) {
	r := newJobRegistry()

	token, _ := r.reserve("a")

	cancelled := false
	if !r.claim("a", token, func() { cancelled = true }) {
		t.Fatal("claim rejected the job that owns the reservation")
	}

	wait := r.cancel("a")
	if !cancelled {
		t.Error("cancel did not cancel the running job")
	}
	if wait == nil {
		t.Fatal("cancel returned no channel to wait on for a running job")
	}
	if !r.canceledByCaller("a", token) {
		t.Error("cancel did not mark the job as superseded by its caller")
	}
	if r.canceledByCaller("b", token) || r.canceledByCaller("a", token+1) {
		t.Error("canceledByCaller reported a job that nobody cancelled")
	}

	select {
	case <-wait:
		t.Fatal("the wait channel closed before the job finished")
	default:
	}

	r.release("a", token)

	select {
	case <-wait:
	default:
		t.Error("release did not wake the caller waiting on the cancelled job")
	}
	if _, ok := r.reserve("a"); !ok {
		t.Error("release did not free the item for a replacement job")
	}
}

func TestRegistryReleaseIgnoresJobThatLostItsSlot(t *testing.T) {
	r := newJobRegistry()

	staleToken, _ := r.reserve("a")

	// The item is cancelled while the job waits and immediately re-queued: the
	// old job may finish its bookkeeping afterwards, but it must not release the
	// reservation that now belongs to its replacement.
	r.cancel("a")
	replacementToken, ok := r.reserve("a")
	if !ok {
		t.Fatal("reserve refused the replacement job")
	}

	r.release("a", staleToken)

	if _, ok := r.reserve("a"); ok {
		t.Error("a stale release retired the replacement job's reservation")
	}

	r.release("a", replacementToken)

	if _, ok := r.reserve("a"); !ok {
		t.Error("releasing the owning job did not free the item")
	}
}
