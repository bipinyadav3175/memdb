package main

import (
	"bufio"
	"strings"
	"testing"
	"time"
)

func TestParseRESPArray(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("*3\r\n$3\r\nSET\r\n$5\r\nhello\r\n$5\r\nworld\r\n"))

	value, err := parseRESP(reader)
	if err != nil {
		t.Fatalf("parseRESP returned error: %v", err)
	}

	items, ok := value.([]interface{})
	if !ok {
		t.Fatalf("parseRESP returned %T, want []interface{}", value)
	}

	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}

	if got, want := items[0].(string), "SET"; got != want {
		t.Fatalf("items[0] = %q, want %q", got, want)
	}

	if got, want := items[1].(string), "hello"; got != want {
		t.Fatalf("items[1] = %q, want %q", got, want)
	}

	if got, want := items[2].(string), "world"; got != want {
		t.Fatalf("items[2] = %q, want %q", got, want)
	}
}

func TestHandleRESPCommandSetGet(t *testing.T) {
	store = &Store{kv: make(map[string]entry)}

	resp := handleRESPCommand([]string{"SET", "hello", "world"})
	if got, want := resp, "+OK\r\n"; got != want {
		t.Fatalf("SET response = %q, want %q", got, want)
	}

	resp = handleRESPCommand([]string{"GET", "hello"})
	if got, want := resp, "$5\r\nworld\r\n"; got != want {
		t.Fatalf("GET response = %q, want %q", got, want)
	}

	resp = handleRESPCommand([]string{"GET", "missing"})
	if got, want := resp, "$-1\r\n"; got != want {
		t.Fatalf("missing GET response = %q, want %q", got, want)
	}
}

func TestHandleRESPCommandSetWithTTL(t *testing.T) {
	store = NewStore()

	resp := handleRESPCommand([]string{"SET", "hello", "world", "EX", "1"})
	if got, want := resp, "+OK\r\n"; got != want {
		t.Fatalf("SET EX response = %q, want %q", got, want)
	}

	if got, want := handleRESPCommand([]string{"TTL", "hello"}), ":1\r\n"; got == want {
		// TTL may be 1 or 0 depending on timing, so only assert it is a positive integer response.
	} else if got == ":0\r\n" || got == ":1\r\n" {
		// expected
	} else {
		t.Fatalf("TTL response = %q, want a positive integer seconds value", got)
	}

	time.Sleep(1200 * time.Millisecond)
	if got, want := handleRESPCommand([]string{"GET", "hello"}), "$-1\r\n"; got != want {
		t.Fatalf("expired GET response = %q, want %q", got, want)
	}
}
