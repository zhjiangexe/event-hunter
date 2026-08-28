// Package canonicaljson implements the RFC 8785 JSON canonicalization behavior
// used by Event Hunter's deterministic hash contracts. It accepts the normal
// JSON value set and normalizes object key order, string escaping and numbers.
package canonicaljson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func Marshal(value any) ([]byte, error) {
	var intermediate bytes.Buffer
	encoder := json.NewEncoder(&intermediate)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(&intermediate)
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var canonical bytes.Buffer
	if err := write(&canonical, document); err != nil {
		return nil, err
	}
	return canonical.Bytes(), nil
}

func SHA256(value any) (string, error) {
	canonical, err := Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func write(target *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		target.WriteString("null")
	case bool:
		target.WriteString(strconv.FormatBool(typed))
	case string:
		encoded, err := quote(typed)
		if err != nil {
			return err
		}
		target.Write(encoded)
	case json.Number:
		text := typed.String()
		if !strings.ContainsAny(text, ".eE") {
			integer, err := strconv.ParseInt(text, 10, 64)
			if err == nil {
				target.WriteString(strconv.FormatInt(integer, 10))
				break
			}
		}
		floating, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return fmt.Errorf("canonical number %q: %w", text, err)
		}
		if floating == 0 {
			target.WriteByte('0')
			break
		}
		formatted := strconv.FormatFloat(floating, 'g', -1, 64)
		formatted = strings.Replace(formatted, "e-0", "e-", 1)
		formatted = strings.Replace(formatted, "e+0", "e+", 1)
		target.WriteString(formatted)
	case []any:
		target.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				target.WriteByte(',')
			}
			if err := write(target, item); err != nil {
				return err
			}
		}
		target.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		target.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				target.WriteByte(',')
			}
			encoded, err := quote(key)
			if err != nil {
				return err
			}
			target.Write(encoded)
			target.WriteByte(':')
			if err := write(target, typed[key]); err != nil {
				return err
			}
		}
		target.WriteByte('}')
	default:
		return fmt.Errorf("unsupported canonical JSON value %T", value)
	}
	return nil
}

func quote(value string) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
