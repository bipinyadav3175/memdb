# memdb

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://go.dev/)
[![Benchmarks](https://img.shields.io/badge/Benchmarks-Run%20Locally-6E6E6E)](#benchmarking)

```text
 ___  ___  ___  ___  ___ 
|  _ \/ _ \/ _ \/ _ \/ __|
| |_) |  __/  __/ (_) \__ \
|____/ \___|\___|\___/|___/

        m e m d b
```

A tiny, Redis-like in-memory key-value store written in Go, exposed over the Redis RESP protocol and backed by TTL-aware entries, append-only persistence, and configurable storage backends.

## Why memdb?

- Redis RESP-compatible commands over TCP
- Expiring keys with TTL and background cleanup
- Append-only file persistence for recovery after restart
- Multiple storage backends: `map`, `syncmap`, and `sharded`
- Small, readable codebase ideal for learning and experimentation

---

## Features

- `SET`, `GET`, `TTL`, `EXPIRE`, and `PING`
- TTL support with `EX` and `PX` units
- Automatic expiration cleanup in a background goroutine
- AOF replay on startup
- Restart-safe TTL replay using original set time + TTL duration
- Pluggable backend selection for different workloads

---

## Quick start

```bash
go run .
```

The server listens on `:5555` by default.

### Select a storage backend

```bash
go run . --store map
go run . --store syncmap
go run . --store sharded
```

You can also set an environment variable:

```bash
MEMORY_DB_STORE=syncmap go run .
```

Available values:

- `map`
- `syncmap`
- `sharded`

---

## RESP commands

### SET

```text
SET mykey myvalue
SET mykey myvalue EX 30
SET mykey myvalue PX 1500
```

### GET

```text
GET mykey
```

### TTL

```text
TTL mykey
```

Returns:

- `-2` if the key does not exist
- `-1` if the key exists but has no expiration
- remaining lifetime in seconds otherwise

### EXPIRE

```text
EXPIRE mykey 30
```

### PING

```text
PING
```

---

## Persistence and recovery

The store writes each mutation to an append-only file (`memory-db.aof` by default) and reloads it on startup.

TTL recovery is implemented carefully:

- the TTL duration is persisted
- the original set time is persisted
- the absolute expiry timestamp is not persisted

This ensures replay restores the correct remaining lifetime rather than resetting the timer on process restart.

---

## Benchmarking

Benchmark coverage is included in the project with a backend matrix for:

- read-heavy access
- write-heavy access
- mixed read/write workloads

### Benchmark results

Fresh results from the local test run on this machine (lower is better):

| Backend | Read-heavy | Write-heavy | Mixed |
| --- | ---: | ---: | ---: |
| map | 44.09 ns/op | 50.00 ns/op | 57.00 ns/op |
| syncmap | 60.38 ns/op | 98.00 ns/op | 20.00 ns/op |
| sharded | 61.54 ns/op | 37.00 ns/op | 40.00 ns/op |

### Benchmark chart

```text
ns/op by workload and backend (lower is better)

Read-heavy
  map      ████████████████████████████ 44.09
  syncmap  ██████████████████████████ 60.38
  sharded  ██████████████████████████ 61.54

Write-heavy
  map      ████████████████████████████ 50.00
  syncmap  ████████████████████████████████████ 98.00
  sharded  ███████████████████████████ 37.00

Mixed
  map      ████████████████████████████ 57.00
  syncmap  ███████████████████████████ 20.00
  sharded  ███████████████████████████ 40.00
```

### Run the benchmark locally

```bash
go test -run '^$' -bench 'BenchmarkStoreBackends' -benchmem
```

### Practical guidance

- `map` is generally fastest for read-heavy workloads
- `sharded` shines for write-heavy access patterns
- `syncmap` can be competitive in mixed workloads depending on contention and access patterns

---

## Project layout

```text
.
├── db.go            # store implementation and persistence logic
├── main.go          # TCP server entrypoint
├── resp.go          # RESP parsing and command handler
├── resp_test.go     # functional tests
├── benchmarks_test.go # benchmark matrix
├── go.mod           # module definition
└── README.md        # project documentation
```

---

## License

This project is licensed under the [MIT License](LICENSE).

---

## Contributing

Feel free to open issues or send pull requests with improvements to:

- TTL behavior
- command coverage
- benchmarking
- persistence semantics
- backend tuning
