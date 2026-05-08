package benchmarks_test

// Batching benchmark simulates a store whose individual Insert has call overhead
// (slice append + counter increment) vs. a single InsertBatch that does one
// append of the whole slice. The real-world gain is much larger when Insert
// carries a network round-trip; this benchmark shows only the structural overhead.

import "testing"

type fakeStore struct {
	items []string
	calls int
}

func (s *fakeStore) Insert(item string) {
	s.items = append(s.items, item)
	s.calls++
}

func (s *fakeStore) InsertBatch(items []string) {
	s.items = append(s.items, items...)
	s.calls++
}

const batchSize = 100

var batchItems = func() []string {
	s := make([]string, batchSize)
	for i := range s {
		s[i] = "item"
	}
	return s
}()

func BenchmarkIndividualInserts(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		st := &fakeStore{items: make([]string, 0, batchSize)}
		for _, item := range batchItems {
			st.Insert(item)
		}
		_ = st
	}
}

func BenchmarkBatchInsert(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		st := &fakeStore{items: make([]string, 0, batchSize)}
		st.InsertBatch(batchItems)
		_ = st
	}
}
