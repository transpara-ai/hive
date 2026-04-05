package sort

import (
	"cmp"
	"math/rand/v2"
	"slices"
	"testing"
)

// --- Timsort public API tests ---

func TestTimsort_Empty(t *testing.T) {
	var data []int
	Timsort(data)
	if len(data) != 0 {
		t.Fatalf("expected empty slice, got %v", data)
	}
}

func TestTimsort_SingleElement(t *testing.T) {
	data := []int{42}
	Timsort(data)
	if data[0] != 42 {
		t.Fatalf("expected [42], got %v", data)
	}
}

func TestTimsort_TwoElements(t *testing.T) {
	data := []int{2, 1}
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_AlreadySorted(t *testing.T) {
	data := makeRange(0, 1000)
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_ReverseSorted(t *testing.T) {
	data := makeRange(0, 1000)
	slices.Reverse(data)
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_AllEqual(t *testing.T) {
	data := make([]int, 500)
	for i := range data {
		data[i] = 7
	}
	Timsort(data)
	for _, v := range data {
		if v != 7 {
			t.Fatalf("expected all 7s, got %v", data)
		}
	}
}

func TestTimsort_RandomSmall(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))
	for trial := 0; trial < 100; trial++ {
		n := rng.IntN(minMerge) + 1
		data := randomInts(rng, n, 100)
		Timsort(data)
		assertSorted(t, data)
	}
}

func TestTimsort_RandomLarge(t *testing.T) {
	rng := rand.New(rand.NewPCG(99, 0))
	sizes := []int{100, 500, 1000, 5000, 10000}
	for _, n := range sizes {
		t.Run("", func(t *testing.T) {
			data := randomInts(rng, n, 1_000_000)
			Timsort(data)
			assertSorted(t, data)
		})
	}
}

func TestTimsort_Duplicates(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 0))
	// Few distinct values — lots of duplicates.
	data := randomInts(rng, 1000, 5)
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_NaturalRuns(t *testing.T) {
	// Build data with natural ascending and descending runs.
	var data []int
	for i := 0; i < 100; i++ {
		data = append(data, i)
	}
	for i := 200; i > 100; i-- {
		data = append(data, i)
	}
	for i := 201; i < 400; i++ {
		data = append(data, i)
	}
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_PipeOrgan(t *testing.T) {
	// Ascending then descending: [0, 1, ..., n/2, n/2-1, ..., 0]
	n := 1000
	data := make([]int, n)
	for i := 0; i < n/2; i++ {
		data[i] = i
		data[n-1-i] = i
	}
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_Sawtooth(t *testing.T) {
	// Repeating ascending runs.
	n := 1000
	data := make([]int, n)
	for i := range data {
		data[i] = i % 50
	}
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_Strings(t *testing.T) {
	data := []string{"banana", "apple", "cherry", "date", "apricot", "blueberry"}
	Timsort(data)
	expected := []string{"apple", "apricot", "banana", "blueberry", "cherry", "date"}
	for i, v := range data {
		if v != expected[i] {
			t.Fatalf("at %d: expected %q, got %q", i, expected[i], v)
		}
	}
}

func TestTimsort_Float64(t *testing.T) {
	data := []float64{3.14, 1.0, 2.71, 0.0, -1.5, 2.71}
	Timsort(data)
	assertSorted(t, data)
}

// --- TimsortFunc tests ---

func TestTimsortFunc_ReverseOrder(t *testing.T) {
	data := []int{1, 5, 3, 2, 4}
	TimsortFunc(data, func(a, b int) int { return cmp.Compare(b, a) })
	// Should be descending.
	for i := 1; i < len(data); i++ {
		if data[i] > data[i-1] {
			t.Fatalf("expected descending order, got %v", data)
		}
	}
}

func TestTimsortFunc_StructByField(t *testing.T) {
	type person struct {
		name string
		age  int
	}
	people := []person{
		{"Alice", 30},
		{"Bob", 25},
		{"Carol", 35},
		{"Dave", 25},
	}
	TimsortFunc(people, func(a, b person) int {
		return cmp.Compare(a.age, b.age)
	})
	for i := 1; i < len(people); i++ {
		if people[i].age < people[i-1].age {
			t.Fatalf("expected sorted by age, got %v", people)
		}
	}
}

// --- Stability test ---

func TestTimsort_Stability(t *testing.T) {
	type item struct {
		key   int
		order int // original position
	}

	rng := rand.New(rand.NewPCG(123, 0))
	n := 2000
	data := make([]item, n)
	for i := range data {
		data[i] = item{key: rng.IntN(20), order: i}
	}

	TimsortFunc(data, func(a, b item) int {
		return cmp.Compare(a.key, b.key)
	})

	for i := 1; i < len(data); i++ {
		if data[i].key < data[i-1].key {
			t.Fatalf("not sorted at index %d", i)
		}
		if data[i].key == data[i-1].key && data[i].order < data[i-1].order {
			t.Fatalf("unstable at index %d: key=%d, orders %d > %d",
				i, data[i].key, data[i-1].order, data[i].order)
		}
	}
}

// --- Internal function tests ---

func TestComputeMinRun(t *testing.T) {
	// For n < minMerge, computeMinRun returns n itself.
	for n := 1; n < minMerge; n++ {
		mr := computeMinRun(n)
		if mr != n {
			t.Fatalf("computeMinRun(%d) = %d, want %d", n, mr, n)
		}
	}

	// For all n >= minMerge, n/minRun should be a power of 2 or close to it.
	// The result should be in [16, 64] (Timsort's standard range).
	for n := minMerge; n <= 100000; n++ {
		mr := computeMinRun(n)
		if mr < 16 || mr > 64 {
			t.Fatalf("computeMinRun(%d) = %d, want [16, 64]", n, mr)
		}
	}

	// Specific known values.
	if mr := computeMinRun(64); mr != 16 {
		t.Fatalf("computeMinRun(64) = %d, want 16", mr)
	}
	if mr := computeMinRun(256); mr != 16 {
		t.Fatalf("computeMinRun(256) = %d, want 16", mr)
	}
	if mr := computeMinRun(100); mr != 25 {
		t.Fatalf("computeMinRun(100) = %d, want 25", mr)
	}
}

func TestCountRunAndMakeAscending_Ascending(t *testing.T) {
	data := []int{1, 2, 3, 4, 5}
	n := countRunAndMakeAscending(data, cmp.Compare)
	if n != 5 {
		t.Fatalf("expected run length 5, got %d", n)
	}
	assertSorted(t, data)
}

func TestCountRunAndMakeAscending_Descending(t *testing.T) {
	data := []int{5, 4, 3, 2, 1}
	n := countRunAndMakeAscending(data, cmp.Compare)
	if n != 5 {
		t.Fatalf("expected run length 5, got %d", n)
	}
	assertSorted(t, data)
}

func TestCountRunAndMakeAscending_SingleElement(t *testing.T) {
	data := []int{42}
	n := countRunAndMakeAscending(data, cmp.Compare)
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
}

func TestCountRunAndMakeAscending_EqualElements(t *testing.T) {
	data := []int{3, 3, 3, 3}
	n := countRunAndMakeAscending(data, cmp.Compare)
	if n != 4 {
		t.Fatalf("expected 4, got %d; data=%v", n, data)
	}
}

func TestCountRunAndMakeAscending_PartialRun(t *testing.T) {
	data := []int{1, 2, 3, 1, 0}
	n := countRunAndMakeAscending(data, cmp.Compare)
	if n != 3 {
		t.Fatalf("expected 3, got %d", n)
	}
}

func TestReverseSlice(t *testing.T) {
	tests := []struct {
		input    []int
		expected []int
	}{
		{[]int{}, []int{}},
		{[]int{1}, []int{1}},
		{[]int{1, 2}, []int{2, 1}},
		{[]int{1, 2, 3, 4, 5}, []int{5, 4, 3, 2, 1}},
	}
	for _, tt := range tests {
		reverseSlice(tt.input)
		for i, v := range tt.input {
			if v != tt.expected[i] {
				t.Fatalf("reverseSlice: expected %v, got %v", tt.expected, tt.input)
			}
		}
	}
}

func TestBinaryInsertionSort(t *testing.T) {
	rng := rand.New(rand.NewPCG(55, 0))
	for trial := 0; trial < 50; trial++ {
		n := rng.IntN(50) + 2
		data := randomInts(rng, n, 1000)
		binaryInsertionSort(data, 0, cmp.Compare)
		assertSorted(t, data)
	}
}

func TestBinaryInsertionSort_WithPresorted(t *testing.T) {
	// First 5 elements already sorted, sort the rest.
	data := []int{1, 3, 5, 7, 9, 2, 4, 6, 8, 10}
	binaryInsertionSort(data, 5, cmp.Compare)
	assertSorted(t, data)
}

func TestGallopRight(t *testing.T) {
	data := []int{1, 3, 5, 7, 9, 11, 13, 15}
	tests := []struct {
		key      int
		hint     int
		expected int
	}{
		{0, 0, 0},   // before all
		{1, 0, 1},   // equal to first (goes right of equal)
		{6, 3, 3},   // between 5 and 7
		{15, 7, 8},  // equal to last
		{20, 7, 8},  // after all
		{7, 3, 4},   // exact match in middle
	}
	for _, tt := range tests {
		got := gallopRight(tt.key, data, tt.hint, cmp.Compare)
		if got != tt.expected {
			t.Errorf("gallopRight(%d, data, %d) = %d, want %d", tt.key, tt.hint, got, tt.expected)
		}
	}
}

func TestGallopLeft(t *testing.T) {
	data := []int{1, 3, 5, 7, 9, 11, 13, 15}
	tests := []struct {
		key      int
		hint     int
		expected int
	}{
		{0, 0, 0},   // before all
		{1, 0, 0},   // equal to first (goes left of equal)
		{6, 3, 3},   // between 5 and 7
		{15, 7, 7},  // equal to last
		{20, 7, 8},  // after all
		{7, 3, 3},   // exact match in middle
	}
	for _, tt := range tests {
		got := gallopLeft(tt.key, data, tt.hint, cmp.Compare)
		if got != tt.expected {
			t.Errorf("gallopLeft(%d, data, %d) = %d, want %d", tt.key, tt.hint, got, tt.expected)
		}
	}
}

// --- Edge cases that exercise merge paths ---

func TestTimsort_TwoRunsMergeLo(t *testing.T) {
	// Create data that will produce exactly two runs where len1 < len2.
	var data []int
	// Run 1: ascending 0..49
	for i := 0; i < 50; i++ {
		data = append(data, i*2) // even: 0, 2, 4, ...
	}
	// Run 2: larger ascending
	for i := 0; i < 100; i++ {
		data = append(data, i*2+1) // odd: 1, 3, 5, ...
	}
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_TwoRunsMergeHi(t *testing.T) {
	// Create data that will produce two runs where len1 > len2.
	var data []int
	for i := 0; i < 100; i++ {
		data = append(data, i*2)
	}
	for i := 0; i < 50; i++ {
		data = append(data, i*2+1)
	}
	Timsort(data)
	assertSorted(t, data)
}

func TestTimsort_ManyShortRuns(t *testing.T) {
	// Force many small runs that need merging.
	rng := rand.New(rand.NewPCG(77, 0))
	n := 5000
	data := make([]int, n)
	for i := 0; i < n; i += 3 {
		end := i + 3
		if end > n {
			end = n
		}
		for j := i; j < end; j++ {
			data[j] = rng.IntN(10000)
		}
		slices.Sort(data[i:end])
	}
	Timsort(data)
	assertSorted(t, data)
}

// --- Comparison with stdlib ---

func TestTimsort_MatchesStdlib(t *testing.T) {
	rng := rand.New(rand.NewPCG(2024, 0))
	for trial := 0; trial < 20; trial++ {
		n := rng.IntN(5000) + 100
		original := randomInts(rng, n, 100000)
		stdlibCopy := make([]int, n)
		copy(stdlibCopy, original)
		timsortCopy := make([]int, n)
		copy(timsortCopy, original)

		slices.Sort(stdlibCopy)
		Timsort(timsortCopy)

		for i := range stdlibCopy {
			if stdlibCopy[i] != timsortCopy[i] {
				t.Fatalf("trial %d: mismatch at %d: stdlib=%d timsort=%d",
					trial, i, stdlibCopy[i], timsortCopy[i])
			}
		}
	}
}

// --- Benchmarks ---

func BenchmarkTimsort_Random1000(b *testing.B) {
	rng := rand.New(rand.NewPCG(1, 0))
	base := randomInts(rng, 1000, 1_000_000)
	data := make([]int, len(base))
	b.ResetTimer()
	for b.Loop() {
		copy(data, base)
		Timsort(data)
	}
}

func BenchmarkTimsort_Sorted1000(b *testing.B) {
	base := makeRange(0, 1000)
	data := make([]int, len(base))
	b.ResetTimer()
	for b.Loop() {
		copy(data, base)
		Timsort(data)
	}
}

func BenchmarkTimsort_Reverse1000(b *testing.B) {
	base := makeRange(0, 1000)
	slices.Reverse(base)
	data := make([]int, len(base))
	b.ResetTimer()
	for b.Loop() {
		copy(data, base)
		Timsort(data)
	}
}

func BenchmarkTimsort_Random10000(b *testing.B) {
	rng := rand.New(rand.NewPCG(2, 0))
	base := randomInts(rng, 10000, 1_000_000)
	data := make([]int, len(base))
	b.ResetTimer()
	for b.Loop() {
		copy(data, base)
		Timsort(data)
	}
}

// --- Helpers ---

func assertSorted[T cmp.Ordered](t *testing.T, data []T) {
	t.Helper()
	for i := 1; i < len(data); i++ {
		if data[i] < data[i-1] {
			t.Fatalf("not sorted at index %d: %v > %v", i, data[i-1], data[i])
		}
	}
}

func randomInts(rng *rand.Rand, n, maxVal int) []int {
	data := make([]int, n)
	for i := range data {
		data[i] = rng.IntN(maxVal)
	}
	return data
}

func makeRange(start, end int) []int {
	data := make([]int, end-start)
	for i := range data {
		data[i] = start + i
	}
	return data
}
