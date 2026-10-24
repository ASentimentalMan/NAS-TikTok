package sampling

import (
	"fmt"
	"testing"
)

func TestReservoirKeepsBoundedUniqueSample(t *testing.T) {
	reservoir, err := New(10)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 1000; index++ {
		reservoir.Add(fmt.Sprint(index))
	}
	items := reservoir.Items()
	if len(items) != 10 {
		t.Fatalf("sample size = %d, want 10", len(items))
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if seen[item] {
			t.Fatalf("duplicate sample item %q", item)
		}
		seen[item] = true
	}
}
