package main

import (
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
}

type Store struct {
	mu sync.RWMutex
	kv map[string]entry
}

var store = NewStore()

func NewStore() *Store {
	s := &Store{kv: make(map[string]entry)}
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

func Get(key string) (string, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	item, ok := store.kv[key]
	if !ok {
		return "", false
	}
	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		delete(store.kv, key)
		return "", false
	}

	return item.value, true
}

func Set(key string, val string) {
	SetWithTTL(key, val, 0)
}

func SetWithTTL(key string, val string, ttl time.Duration) {
	store.mu.Lock()
	defer store.mu.Unlock()

	item := entry{value: val}
	if ttl > 0 {
		item.expiresAt = time.Now().Add(ttl)
	}
	store.kv[key] = item
}

func Expire(key string, ttl time.Duration) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	item, ok := store.kv[key]
	if !ok {
		return false
	}
	if ttl <= 0 {
		delete(store.kv, key)
		return true
	}

	item.expiresAt = time.Now().Add(ttl)
	store.kv[key] = item
	return true
}

func TTL(key string) int64 {
	store.mu.Lock()
	defer store.mu.Unlock()

	item, ok := store.kv[key]
	if !ok {
		return -2
	}
	if item.expiresAt.IsZero() {
		return -1
	}

	remaining := time.Until(item.expiresAt)
	if remaining <= 0 {
		delete(store.kv, key)
		return -2
	}

	return int64(remaining / time.Second)
}
