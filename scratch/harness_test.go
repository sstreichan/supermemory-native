package main

import "testing"

func TestHarness(t *testing.T) {
	stats := &TestStats{}
	testExtremeInputsAndBoundaries(stats)
	testMaliciousSQLInjection(stats)
	testLargeVolumeAndPagination(stats)
	testVaultHydrationAndFallbacks(stats)
	testConcurrencyStress(stats)

	if stats.Failed > 0 {
		t.Fatalf("Empirical test suite failed with %d failures (%d passed)", stats.Failed, stats.Passed)
	}
}
