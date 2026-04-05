// Package sort implements Timsort, a hybrid stable sorting algorithm that
// combines merge sort and insertion sort. Timsort exploits existing order
// (natural runs) in data, making it efficient for real-world inputs.
//
// Algorithm overview:
//  1. Scan for natural runs (ascending or descending sequences)
//  2. Extend short runs to a minimum length using binary insertion sort
//  3. Merge runs using a merge stack with invariants that guarantee O(n log n)
//  4. Use galloping mode during merges to skip large blocks efficiently
package sort

import "cmp"

const (
	// minMerge is the minimum array size that triggers the full Timsort
	// algorithm. Below this threshold, binary insertion sort is used directly.
	minMerge = 32

	// minGallop is the initial threshold for entering galloping mode during
	// merges. When one run "wins" this many times consecutively, we switch
	// to exponential search (galloping) to skip elements faster.
	minGallop = 7
)

// run represents a naturally ordered subsequence within the input slice.
type run struct {
	start int
	len   int
}

// Timsort sorts a slice of any ordered type using the Timsort algorithm.
// The sort is stable: equal elements preserve their original order.
func Timsort[T cmp.Ordered](data []T) {
	TimsortFunc(data, cmp.Compare)
}

// TimsortFunc sorts a slice using a custom comparison function.
// The comparison function should return negative if a < b, zero if a == b,
// and positive if a > b. The sort is stable.
func TimsortFunc[T any](data []T, compare func(a, b T) int) {
	n := len(data)
	if n < 2 {
		return
	}

	// For small arrays, binary insertion sort is sufficient.
	if n < minMerge {
		initRunLen := countRunAndMakeAscending(data, compare)
		binaryInsertionSort(data, initRunLen, compare)
		return
	}

	ts := &timsortState[T]{
		data:       data,
		compare:    compare,
		minGallop:  minGallop,
		runStack:   make([]run, 0, 40), // sized for worst case
		tmp:        nil,
	}

	minRun := computeMinRun(n)

	lo := 0
	remaining := n
	for remaining > 0 {
		// Identify the next natural run.
		runLen := countRunAndMakeAscending(data[lo:], compare)

		// Extend short runs to minRun using binary insertion sort.
		if runLen < minRun {
			force := remaining
			if force > minRun {
				force = minRun
			}
			binaryInsertionSort(data[lo:lo+force], runLen, compare)
			runLen = force
		}

		// Push this run onto the stack and merge to maintain invariants.
		ts.pushRun(run{start: lo, len: runLen})
		ts.mergeCollapse()

		lo += runLen
		remaining -= runLen
	}

	// Merge all remaining runs to complete the sort.
	ts.mergeForceCollapse()
}

// timsortState holds the merge state for a single Timsort operation.
type timsortState[T any] struct {
	data      []T
	compare   func(a, b T) int
	minGallop int
	runStack  []run
	tmp       []T // temporary buffer for merges
}

// computeMinRun returns the minimum run length for Timsort. It chooses a
// value in [32, 64] such that n/minRun is a power of 2 or close to it,
// which keeps merge tree balanced.
func computeMinRun(n int) int {
	r := 0
	for n >= minMerge {
		r |= n & 1
		n >>= 1
	}
	return n + r
}

// countRunAndMakeAscending finds the length of the natural run starting at
// the beginning of data. If the run is descending (strictly decreasing),
// it reverses it in place. Returns the run length.
func countRunAndMakeAscending[T any](data []T, compare func(a, b T) int) int {
	n := len(data)
	if n <= 1 {
		return n
	}

	runLen := 2
	if compare(data[1], data[0]) < 0 {
		// Descending run — find its extent, then reverse.
		for runLen < n && compare(data[runLen], data[runLen-1]) < 0 {
			runLen++
		}
		reverseSlice(data[:runLen])
	} else {
		// Ascending run (including equal elements for stability).
		for runLen < n && compare(data[runLen], data[runLen-1]) >= 0 {
			runLen++
		}
	}
	return runLen
}

// reverseSlice reverses the elements of data in place.
func reverseSlice[T any](data []T) {
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
}

// binaryInsertionSort sorts data[0:len(data)] using binary insertion sort.
// It assumes data[0:start] is already sorted. This is efficient for small
// arrays and nearly-sorted data.
func binaryInsertionSort[T any](data []T, start int, compare func(a, b T) int) {
	if start == 0 {
		start = 1
	}
	for i := start; i < len(data); i++ {
		pivot := data[i]

		// Binary search for insertion point.
		lo, hi := 0, i
		for lo < hi {
			mid := lo + (hi-lo)/2
			if compare(pivot, data[mid]) < 0 {
				hi = mid
			} else {
				lo = mid + 1
			}
		}

		// Shift elements to make room and insert.
		copy(data[lo+1:i+1], data[lo:i])
		data[lo] = pivot
	}
}

// pushRun adds a run to the pending-merge stack.
func (ts *timsortState[T]) pushRun(r run) {
	ts.runStack = append(ts.runStack, r)
}

// mergeCollapse merges adjacent runs on the stack until the Timsort
// invariants are satisfied:
//   - runStack[i-2].len > runStack[i-1].len + runStack[i].len
//   - runStack[i-1].len > runStack[i].len
//
// These invariants ensure balanced merges and O(n log n) performance.
func (ts *timsortState[T]) mergeCollapse() {
	for len(ts.runStack) > 1 {
		n := len(ts.runStack) - 2

		if n > 0 && ts.runStack[n-1].len <= ts.runStack[n].len+ts.runStack[n+1].len {
			if ts.runStack[n-1].len < ts.runStack[n+1].len {
				n--
			}
			ts.mergeAt(n)
		} else if ts.runStack[n].len <= ts.runStack[n+1].len {
			ts.mergeAt(n)
		} else {
			break
		}
	}
}

// mergeForceCollapse merges all remaining runs on the stack into one.
func (ts *timsortState[T]) mergeForceCollapse() {
	for len(ts.runStack) > 1 {
		n := len(ts.runStack) - 2
		if n > 0 && ts.runStack[n-1].len < ts.runStack[n+1].len {
			n--
		}
		ts.mergeAt(n)
	}
}

// mergeAt merges runs at stack positions i and i+1.
func (ts *timsortState[T]) mergeAt(i int) {
	base1 := ts.runStack[i].start
	len1 := ts.runStack[i].len
	base2 := ts.runStack[i+1].start
	len2 := ts.runStack[i+1].len

	// Record merged length and remove the second run from the stack.
	ts.runStack[i].len = len1 + len2
	if i == len(ts.runStack)-3 {
		ts.runStack[i+1] = ts.runStack[i+2]
	}
	ts.runStack = ts.runStack[:len(ts.runStack)-1]

	// Find where the first element of run2 goes in run1 using galloping.
	k := gallopRight(ts.data[base2], ts.data[base1:base1+len1], 0, ts.compare)
	base1 += k
	len1 -= k
	if len1 == 0 {
		return
	}

	// Find where the last element of run1 goes in run2 using galloping.
	len2 = gallopLeft(ts.data[base1+len1-1], ts.data[base2:base2+len2], len2-1, ts.compare)
	if len2 == 0 {
		return
	}

	// Merge the remaining parts, choosing the smaller run for temp storage.
	if len1 <= len2 {
		ts.mergeLo(base1, len1, base2, len2)
	} else {
		ts.mergeHi(base1, len1, base2, len2)
	}
}

// gallopRight finds the position in a sorted slice where key would be
// inserted to the right of any equal elements. Uses exponential search
// starting from hint, then binary search within the found range.
func gallopRight[T any](key T, data []T, hint int, compare func(a, b T) int) int {
	n := len(data)
	lastOfs := 0
	ofs := 1

	if compare(key, data[hint]) < 0 {
		// Gallop left from hint.
		maxOfs := hint + 1
		for ofs < maxOfs && compare(key, data[hint-ofs]) < 0 {
			lastOfs = ofs
			ofs = ofs*2 + 1
			if ofs <= 0 { // overflow
				ofs = maxOfs
			}
		}
		if ofs > maxOfs {
			ofs = maxOfs
		}
		tmp := lastOfs
		lastOfs = hint - ofs
		ofs = hint - tmp
	} else {
		// Gallop right from hint.
		maxOfs := n - hint
		for ofs < maxOfs && compare(key, data[hint+ofs]) >= 0 {
			lastOfs = ofs
			ofs = ofs*2 + 1
			if ofs <= 0 { // overflow
				ofs = maxOfs
			}
		}
		if ofs > maxOfs {
			ofs = maxOfs
		}
		lastOfs += hint
		ofs += hint
	}

	// Binary search within [lastOfs+1, ofs).
	lastOfs++
	for lastOfs < ofs {
		mid := lastOfs + (ofs-lastOfs)/2
		if compare(key, data[mid]) < 0 {
			ofs = mid
		} else {
			lastOfs = mid + 1
		}
	}
	return ofs
}

// gallopLeft finds the position in a sorted slice where key would be
// inserted to the left of any equal elements. Uses exponential search
// starting from hint, then binary search.
func gallopLeft[T any](key T, data []T, hint int, compare func(a, b T) int) int {
	n := len(data)
	lastOfs := 0
	ofs := 1

	if compare(key, data[hint]) > 0 {
		// Gallop right from hint.
		maxOfs := n - hint
		for ofs < maxOfs && compare(key, data[hint+ofs]) > 0 {
			lastOfs = ofs
			ofs = ofs*2 + 1
			if ofs <= 0 { // overflow
				ofs = maxOfs
			}
		}
		if ofs > maxOfs {
			ofs = maxOfs
		}
		lastOfs += hint
		ofs += hint
	} else {
		// Gallop left from hint.
		maxOfs := hint + 1
		for ofs < maxOfs && compare(key, data[hint-ofs]) <= 0 {
			lastOfs = ofs
			ofs = ofs*2 + 1
			if ofs <= 0 { // overflow
				ofs = maxOfs
			}
		}
		if ofs > maxOfs {
			ofs = maxOfs
		}
		tmp := lastOfs
		lastOfs = hint - ofs
		ofs = hint - tmp
	}

	// Binary search within [lastOfs+1, ofs).
	lastOfs++
	for lastOfs < ofs {
		mid := lastOfs + (ofs-lastOfs)/2
		if compare(key, data[mid]) <= 0 {
			ofs = mid
		} else {
			lastOfs = mid + 1
		}
	}
	return ofs
}

// ensureTmp ensures the temporary buffer is at least minLen in capacity.
func (ts *timsortState[T]) ensureTmp(minLen int) []T {
	if len(ts.tmp) < minLen {
		ts.tmp = make([]T, minLen)
	}
	return ts.tmp[:minLen]
}

// mergeLo merges two adjacent runs where len1 <= len2.
// Copies run1 into temp and merges left to right.
func (ts *timsortState[T]) mergeLo(base1, len1, base2, len2 int) {
	tmp := ts.ensureTmp(len1)
	copy(tmp, ts.data[base1:base1+len1])

	cursor1 := 0       // index into tmp (run1)
	cursor2 := base2    // index into data (run2)
	dest := base1       // destination in data
	minGallop := ts.minGallop

	for {
		count1 := 0 // consecutive wins for run1
		count2 := 0 // consecutive wins for run2

		// Straight merge until one run starts dominating.
		for {
			if compare := ts.compare(tmp[cursor1], ts.data[cursor2]); compare <= 0 {
				ts.data[dest] = tmp[cursor1]
				dest++
				cursor1++
				count1++
				count2 = 0
				len1--
				if len1 == 0 {
					return
				}
			} else {
				ts.data[dest] = ts.data[cursor2]
				dest++
				cursor2++
				count2++
				count1 = 0
				len2--
				if len2 == 0 {
					copy(ts.data[dest:], tmp[cursor1:cursor1+len1])
					return
				}
			}
			if count1|count2 >= minGallop {
				break
			}
		}

		// Galloping mode — one run is winning consistently.
		for {
			count1 = gallopRight(ts.data[cursor2], tmp[cursor1:cursor1+len1], 0, ts.compare)
			if count1 > 0 {
				copy(ts.data[dest:], tmp[cursor1:cursor1+count1])
				dest += count1
				cursor1 += count1
				len1 -= count1
				if len1 == 0 {
					return
				}
			}
			ts.data[dest] = ts.data[cursor2]
			dest++
			cursor2++
			len2--
			if len2 == 0 {
				copy(ts.data[dest:], tmp[cursor1:cursor1+len1])
				return
			}

			count2 = gallopLeft(tmp[cursor1], ts.data[cursor2:cursor2+len2], 0, ts.compare)
			if count2 > 0 {
				copy(ts.data[dest:], ts.data[cursor2:cursor2+count2])
				dest += count2
				cursor2 += count2
				len2 -= count2
				if len2 == 0 {
					copy(ts.data[dest:], tmp[cursor1:cursor1+len1])
					return
				}
			}
			ts.data[dest] = tmp[cursor1]
			dest++
			cursor1++
			len1--
			if len1 == 0 {
				return
			}

			minGallop--
			if count1 < minGallop && count2 < minGallop {
				break
			}
		}

		if minGallop < 0 {
			minGallop = 0
		}
		minGallop += 2 // penalize exit from galloping
		ts.minGallop = minGallop
	}
}

// mergeHi merges two adjacent runs where len1 > len2.
// Copies run2 into temp and merges right to left.
func (ts *timsortState[T]) mergeHi(base1, len1, base2, len2 int) {
	tmp := ts.ensureTmp(len2)
	copy(tmp, ts.data[base2:base2+len2])

	cursor1 := base1 + len1 - 1 // index into data (run1), from right
	cursor2 := len2 - 1          // index into tmp (run2), from right
	dest := base2 + len2 - 1     // destination, from right
	minGallop := ts.minGallop

	for {
		count1 := 0
		count2 := 0

		// Straight merge from right.
		for {
			if ts.compare(tmp[cursor2], ts.data[cursor1]) >= 0 {
				ts.data[dest] = tmp[cursor2]
				dest--
				cursor2--
				count2++
				count1 = 0
				len2--
				if len2 == 0 {
					return
				}
			} else {
				ts.data[dest] = ts.data[cursor1]
				dest--
				cursor1--
				count1++
				count2 = 0
				len1--
				if len1 == 0 {
					copy(ts.data[dest-len2+1:], tmp[:len2])
					return
				}
			}
			if count1|count2 >= minGallop {
				break
			}
		}

		// Galloping mode from right.
		for {
			count1 = len1 - gallopRight(tmp[cursor2], ts.data[base1:base1+len1], len1-1, ts.compare)
			if count1 > 0 {
				dest -= count1
				cursor1 -= count1
				len1 -= count1
				copy(ts.data[dest+1:], ts.data[cursor1+1:cursor1+1+count1])
				if len1 == 0 {
					copy(ts.data[dest-len2+1:], tmp[:len2])
					return
				}
			}
			ts.data[dest] = tmp[cursor2]
			dest--
			cursor2--
			len2--
			if len2 == 0 {
				return
			}

			count2 = len2 - gallopLeft(ts.data[cursor1], tmp[:len2], len2-1, ts.compare)
			if count2 > 0 {
				dest -= count2
				cursor2 -= count2
				len2 -= count2
				copy(ts.data[dest+1:], tmp[cursor2+1:cursor2+1+count2])
				if len2 == 0 {
					return
				}
			}
			ts.data[dest] = ts.data[cursor1]
			dest--
			cursor1--
			len1--
			if len1 == 0 {
				copy(ts.data[dest-len2+1:], tmp[:len2])
				return
			}

			minGallop--
			if count1 < minGallop && count2 < minGallop {
				break
			}
		}

		if minGallop < 0 {
			minGallop = 0
		}
		minGallop += 2
		ts.minGallop = minGallop
	}
}
