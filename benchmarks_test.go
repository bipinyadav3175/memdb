package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

func benchmarkReadHeavy(b *testing.B, s *Store) {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
		s.Set(keys[i], "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Get(keys[i%len(keys)])
	}
}

func benchmarkWriteHeavy(b *testing.B, s *Store) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Set(fmt.Sprintf("key-%d", i%10000), "value")
	}
}

func benchmarkMixed(b *testing.B, s *Store) {
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
		s.Set(keys[i], "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%4 == 0 {
			s.Set(fmt.Sprintf("key-%d", i%1000), "new-value")
			continue
		}
		_, _ = s.Get(keys[i%len(keys)])
	}
}

func BenchmarkStoreBackends(b *testing.B) {
	cases := []struct {
		backend string
		kind    storeKind
		name    string
		work    func(*testing.B, *Store)
	}{
		{backend: "map", kind: storeKindMap, name: "read_heavy", work: benchmarkReadHeavy},
		{backend: "map", kind: storeKindMap, name: "write_heavy", work: benchmarkWriteHeavy},
		{backend: "map", kind: storeKindMap, name: "mixed", work: benchmarkMixed},
		{backend: "syncmap", kind: storeKindSyncMap, name: "read_heavy", work: benchmarkReadHeavy},
		{backend: "syncmap", kind: storeKindSyncMap, name: "write_heavy", work: benchmarkWriteHeavy},
		{backend: "syncmap", kind: storeKindSyncMap, name: "mixed", work: benchmarkMixed},
		{backend: "sharded", kind: storeKindSharded, name: "read_heavy", work: benchmarkReadHeavy},
		{backend: "sharded", kind: storeKindSharded, name: "write_heavy", work: benchmarkWriteHeavy},
		{backend: "sharded", kind: storeKindSharded, name: "mixed", work: benchmarkMixed},
	}

	for _, tc := range cases {
		b.Run(fmt.Sprintf("%s/%s", tc.backend, tc.name), func(b *testing.B) {
			s := newStoreWithKind(tc.kind, filepath.Join(b.TempDir(), "bench.aof"))
			tc.work(b, s)
		})
	}
}
