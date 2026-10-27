package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
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

type Store struct {
	mu      sync.RWMutex
	kv      map[string]entry
	aofPath string
}

var store = NewStore()

func defaultAOFPath() string {
	if path := os.Getenv("MEMORY_DB_AOF_PATH"); path != "" {
		return path
	}
	return "memory-db.aof"
}

func NewStore() *Store {
	return NewStoreWithPath(defaultAOFPath())
}

func NewStoreWithPath(path string) *Store {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			panic(err)
		}
	}

	s := &Store{
		kv:      make(map[string]entry),
		aofPath: path,
	}
	s.loadFromFile()
	go s.cleanupLoop()
	return s
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for key, item := range s.kv {
			if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
				delete(s.kv, key)
			}
		}
		s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()

	switch rec.Op {
	case "DEL":
		delete(s.kv, rec.Key)
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
		s.kv[rec.Key] = item
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
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.kv[key]
	if !ok {
		return "", false
	}
	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		delete(s.kv, key)
		return "", false
	}

	return item.value, true
}

func (s *Store) Set(key string, val string) {
	s.SetWithTTL(key, val, 0)
}

func (s *Store) SetWithTTL(key string, val string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	item := entry{value: val, setAt: now}
	if ttl > 0 {
		item.expiresAt = now.Add(ttl)
	}
	s.kv[key] = item
	s.persistRecord("SET", key, val, ttl, now)
}

func (s *Store) Expire(key string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.kv[key]
	if !ok {
		return false
	}
	if ttl <= 0 {
		delete(s.kv, key)
		s.persistRecord("DEL", key, "", 0, time.Time{})
		return true
	}

	now := time.Now()
	item.expiresAt = now.Add(ttl)
	item.setAt = now
	s.kv[key] = item
	s.persistRecord("SET", key, item.value, ttl, now)
	return true
}

func (s *Store) TTL(key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.kv[key]
	if !ok {
		return -2
	}
	if item.expiresAt.IsZero() {
		return -1
	}

	remaining := time.Until(item.expiresAt)
	if remaining <= 0 {
		delete(s.kv, key)
		return -2
	}

	return int64(remaining / time.Second)
}

func Get(key string) (string, bool) {
	return store.Get(key)
}

func Set(key string, val string) {
	store.Set(key, val)
}

func SetWithTTL(key string, val string, ttl time.Duration) {
	store.SetWithTTL(key, val, ttl)
}

func Expire(key string, ttl time.Duration) bool {
	return store.Expire(key, ttl)
}

func TTL(key string) int64 {
	return store.TTL(key)
}
