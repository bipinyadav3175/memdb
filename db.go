package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
	setAt     time.Time
}

type persistedRecord struct {
	Op    string `json:"op"`
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
	TTLMS int64  `json:"ttl_ms,omitempty"`
	SetAt int64  `json:"set_at_unix_nano,omitempty"`
}

type storeKind string

const (
	storeKindMap     storeKind = "map"
	storeKindSyncMap storeKind = "syncmap"
	storeKindSharded storeKind = "sharded"
)

type shardedMap struct {
	mu sync.RWMutex
	kv map[string]entry
}

type Store struct {
	mu      sync.RWMutex
	kv      map[string]entry
	syncMap *sync.Map
	shards  []*shardedMap
	aofPath string
	kind    storeKind
}

var store *Store

func defaultAOFPath() string {
	if path := os.Getenv("MEMORY_DB_AOF_PATH"); path != "" {
		return path
	}
	return "memory-db.aof"
}

func newStoreWithKind(kind storeKind, path string) *Store {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			panic(err)
		}
	}

	s := &Store{
		aofPath: path,
		kind:    kind,
	}

	switch kind {
	case storeKindSyncMap:
		s.syncMap = &sync.Map{}
	case storeKindSharded:
		s.shards = make([]*shardedMap, 32)
		for i := range s.shards {
			s.shards[i] = &shardedMap{kv: make(map[string]entry)}
		}
	default:
		s.kv = make(map[string]entry)
	}

	s.loadFromFile()
	go s.cleanupLoop()
	return s
}

func defaultStoreKind() storeKind {
	kind := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_DB_STORE")))
	if kind == "" {
		return storeKindMap
	}

	switch kind {
	case "syncmap", "sync_map", "sync-map":
		return storeKindSyncMap
	case "sharded", "shard":
		return storeKindSharded
	case "map":
		return storeKindMap
	default:
		return storeKindMap
	}
}

func ParseStoreKind(raw string) (storeKind, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	switch s {
	case "", "map":
		return storeKindMap, nil
	case "syncmap", "sync_map", "sync-map":
		return storeKindSyncMap, nil
	case "sharded", "shard":
		return storeKindSharded, nil
	default:
		return "", fmt.Errorf("unsupported store kind %q (choose map, syncmap or sharded)", raw)
	}
}

func NewStore() *Store {
	return newStoreWithKind(defaultStoreKind(), defaultAOFPath())
}

func NewStoreWithPath(path string) *Store {
	return newStoreWithKind(defaultStoreKind(), path)
}

func (s *Store) kindFor() storeKind {
	if s == nil || s.kind == "" {
		return storeKindMap
	}
	return s.kind
}

func (s *Store) shardFor(key string) *shardedMap {
	if len(s.shards) == 0 {
		return nil
	}
	index := 0
	for _, ch := range key {
		index = (index + int(ch)) % len(s.shards)
	}
	return s.shards[index]
}

func (s *Store) getEntry(key string) (entry, bool) {
	switch s.kindFor() {
	case storeKindSyncMap:
		value, ok := s.syncMap.Load(key)
		if !ok {
			return entry{}, false
		}
		item, ok := value.(entry)
		return item, ok
	case storeKindSharded:
		shard := s.shardFor(key)
		if shard == nil {
			return entry{}, false
		}
		shard.mu.RLock()
		defer shard.mu.RUnlock()
		item, ok := shard.kv[key]
		return item, ok
	default:
		s.mu.RLock()
		defer s.mu.RUnlock()
		item, ok := s.kv[key]
		return item, ok
	}
}

func (s *Store) setEntry(key string, item entry) {
	switch s.kindFor() {
	case storeKindSyncMap:
		s.syncMap.Store(key, item)
	case storeKindSharded:
		shard := s.shardFor(key)
		if shard == nil {
			return
		}
		shard.mu.Lock()
		shard.kv[key] = item
		shard.mu.Unlock()
	default:
		s.mu.Lock()
		s.kv[key] = item
		s.mu.Unlock()
	}
}

func (s *Store) deleteEntry(key string) {
	switch s.kindFor() {
	case storeKindSyncMap:
		s.syncMap.Delete(key)
	case storeKindSharded:
		shard := s.shardFor(key)
		if shard == nil {
			return
		}
		shard.mu.Lock()
		delete(shard.kv, key)
		shard.mu.Unlock()
	default:
		s.mu.Lock()
		delete(s.kv, key)
		s.mu.Unlock()
	}
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		switch s.kindFor() {
		case storeKindSyncMap:
			s.syncMap.Range(func(key, value any) bool {
				item, ok := value.(entry)
				if !ok {
					return true
				}
				if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
					s.syncMap.Delete(key)
				}
				return true
			})
		case storeKindSharded:
			for _, shard := range s.shards {
				shard.mu.Lock()
				for key, item := range shard.kv {
					if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
						delete(shard.kv, key)
					}
				}
				shard.mu.Unlock()
			}
		default:
			s.mu.Lock()
			for key, item := range s.kv {
				if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
					delete(s.kv, key)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *Store) loadFromFile() {
	if s.aofPath == "" {
		return
	}

	file, err := os.Open(s.aofPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var rec persistedRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		s.applyRecord(rec)
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

func (s *Store) applyRecord(rec persistedRecord) {
	switch rec.Op {
	case "DEL":
		s.deleteEntry(rec.Key)
	case "SET", "":
		item := entry{value: rec.Value}
		if rec.TTLMS > 0 {
			if rec.SetAt > 0 {
				setAt := time.Unix(0, rec.SetAt)
				remaining := time.Duration(rec.TTLMS)*time.Millisecond - time.Since(setAt)
				if remaining > 0 {
					item.expiresAt = time.Now().Add(remaining)
					item.setAt = setAt
				}
			} else {
				item.expiresAt = time.Now().Add(time.Duration(rec.TTLMS) * time.Millisecond)
			}
		}
		s.setEntry(rec.Key, item)
	}
}

func (s *Store) persistRecord(op, key, value string, ttl time.Duration, setAt time.Time) {
	if s.aofPath == "" {
		return
	}

	file, err := os.OpenFile(s.aofPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()

	rec := persistedRecord{Op: op, Key: key, Value: value}
	if ttl > 0 {
		rec.TTLMS = ttl.Milliseconds()
		rec.SetAt = setAt.UnixNano()
	}

	payload, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_, _ = file.Write(append(payload, '\n'))
}

func (s *Store) Get(key string) (string, bool) {
	item, ok := s.getEntry(key)
	if !ok {
		return "", false
	}
	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		s.deleteEntry(key)
		return "", false
	}

	return item.value, true
}

func (s *Store) Set(key string, val string) {
	s.SetWithTTL(key, val, 0)
}

func (s *Store) SetWithTTL(key string, val string, ttl time.Duration) {
	now := time.Now()
	item := entry{value: val, setAt: now}
	if ttl > 0 {
		item.expiresAt = now.Add(ttl)
	}
	s.setEntry(key, item)
	s.persistRecord("SET", key, val, ttl, now)
}

func (s *Store) Expire(key string, ttl time.Duration) bool {
	item, ok := s.getEntry(key)
	if !ok {
		return false
	}
	if ttl <= 0 {
		s.deleteEntry(key)
		s.persistRecord("DEL", key, "", 0, time.Time{})
		return true
	}

	now := time.Now()
	item.expiresAt = now.Add(ttl)
	item.setAt = now
	s.setEntry(key, item)
	s.persistRecord("SET", key, item.value, ttl, now)
	return true
}

func (s *Store) TTL(key string) int64 {
	item, ok := s.getEntry(key)
	if !ok {
		return -2
	}
	if item.expiresAt.IsZero() {
		return -1
	}

	remaining := time.Until(item.expiresAt)
	if remaining <= 0 {
		s.deleteEntry(key)
		return -2
	}

	return int64(remaining / time.Second)
}

func ensureStore() *Store {
	if store == nil {
		store = NewStore()
	}
	return store
}

func Get(key string) (string, bool) {
	return ensureStore().Get(key)
}

func Set(key string, val string) {
	ensureStore().Set(key, val)
}

func SetWithTTL(key string, val string, ttl time.Duration) {
	ensureStore().SetWithTTL(key, val, ttl)
}

func Expire(key string, ttl time.Duration) bool {
	return ensureStore().Expire(key, ttl)
}

func TTL(key string) int64 {
	return ensureStore().TTL(key)
}
