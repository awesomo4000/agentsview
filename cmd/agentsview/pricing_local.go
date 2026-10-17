package main

func runPricingExclusive(
	runner remoteSyncExclusiveRunner,
	work func() error,
) error {
	if runner == nil {
		return work()
	}
	return runner.RunExclusive(work)
}
