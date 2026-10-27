package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

func parseRESP(reader *bufio.Reader) (interface{}, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	switch prefix {
	case '+':
		line, err := readRESPLine(reader)
		if err != nil {
			return nil, err
		}
		return line, nil
	case '-':
		line, err := readRESPLine(reader)
		if err != nil {
			return nil, err
		}
		return line, nil
	case ':':
		line, err := readRESPLine(reader)
		if err != nil {
			return nil, err
		}
		return strconv.ParseInt(line, 10, 64)
	case '$':
		line, err := readRESPLine(reader)
		if err != nil {
			return nil, err
		}

		length, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("invalid bulk string length: %w", err)
		}
		if length < 0 {
			return nil, nil
		}

		buf := make([]byte, length)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, err
		}
		if _, err := reader.Discard(2); err != nil {
			return nil, err
		}
		return string(buf), nil
	case '*':
		line, err := readRESPLine(reader)
		if err != nil {
			return nil, err
		}

		count, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("invalid array length: %w", err)
		}
		if count < 0 {
			return nil, nil
		}

		items := make([]interface{}, 0, count)
		for i := 0; i < count; i++ {
			item, err := parseRESP(reader)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported RESP type %q", string(prefix))
	}
}

func readRESPLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", fmt.Errorf("invalid RESP line: %q", line)
	}
	return line[:len(line)-2], nil
}

func toStrings(items []interface{}) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			out = append(out, value)
		case []byte:
			out = append(out, string(value))
		default:
			out = append(out, fmt.Sprintf("%v", value))
		}
	}
	return out
}

func parseTTL(unit, value string) (time.Duration, error) {
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("ttl must be positive")
	}

	switch strings.ToUpper(unit) {
	case "EX":
		return time.Duration(seconds) * time.Second, nil
	case "PX":
		return time.Duration(seconds) * time.Millisecond, nil
	default:
		return 0, fmt.Errorf("unsupported ttl unit %q", unit)
	}
}

func handleRESPCommand(args []string) string {
	if len(args) == 0 {
		return "-ERR empty command\r\n"
	}

	switch strings.ToUpper(args[0]) {
	case "PING":
		return "+PONG\r\n"
	case "SET":
		if len(args) == 3 {
			Set(args[1], args[2])
			return "+OK\r\n"
		}
		if len(args) == 5 {
			ttl, err := parseTTL(args[3], args[4])
			if err != nil {
				return "-ERR invalid ttl value\r\n"
			}
			SetWithTTL(args[1], args[2], ttl)
			return "+OK\r\n"
		}
		return "-ERR wrong number of arguments for 'set' command\r\n"
	case "GET":
		if len(args) != 2 {
			return "-ERR wrong number of arguments for 'get' command\r\n"
		}
		value, ok := Get(args[1])
		if !ok {
			return "$-1\r\n"
		}
		return fmt.Sprintf("$%d\r\n%s\r\n", len(value), value)
	case "TTL":
		if len(args) != 2 {
			return "-ERR wrong number of arguments for 'ttl' command\r\n"
		}
		ttl := TTL(args[1])
		if ttl == -2 {
			return ":-2\r\n"
		}
		if ttl == -1 {
			return ":-1\r\n"
		}
		return fmt.Sprintf(":%d\r\n", ttl)
	case "EXPIRE":
		if len(args) != 3 {
			return "-ERR wrong number of arguments for 'expire' command\r\n"
		}
		seconds, err := strconv.Atoi(args[2])
		if err != nil || seconds <= 0 {
			return "-ERR invalid expire value\r\n"
		}
		if Expire(args[1], time.Duration(seconds)*time.Second) {
			return ":1\r\n"
		}
		return ":0\r\n"
	default:
		return "-ERR unknown command\r\n"
	}
}
