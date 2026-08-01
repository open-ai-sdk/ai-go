package aisdk

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// HashCanonical hashes JSON using JavaScript JSON.stringify semantics after
// recursively sorting object keys by UTF-16 code units.
func HashCanonical(raw json.RawMessage) (string, error) {
	canonical, err := canonicalApprovalJSON(raw)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func canonicalApprovalJSON(raw []byte) ([]byte, error) {
	if err := validateApprovalUnicode(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	var output bytes.Buffer
	if err := writeCanonicalApprovalJSON(&output, value); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func validateApprovalUnicode(raw []byte) error {
	if !utf8.Valid(raw) {
		return errors.New("approval input is not valid UTF-8")
	}
	inString := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || i+1 >= len(raw) || raw[i+1] != 'u' {
				i++
				continue
			}
			if i+6 > len(raw) {
				return errors.New("incomplete unicode escape")
			}
			unit, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
			if err != nil {
				return fmt.Errorf("invalid unicode escape: %w", err)
			}
			if 0xDC00 <= unit && unit <= 0xDFFF {
				return errors.New("unpaired low surrogate")
			}
			if 0xD800 <= unit && unit <= 0xDBFF {
				if i+12 > len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
					return errors.New("unpaired high surrogate")
				}
				low, err := strconv.ParseUint(string(raw[i+8:i+12]), 16, 16)
				if err != nil || low < 0xDC00 || low > 0xDFFF {
					return errors.New("unpaired high surrogate")
				}
				i += 11
				continue
			}
			i += 5
		}
	}
	return nil
}

func writeCanonicalApprovalJSON(output *bytes.Buffer, value any) error {
	switch value := value.(type) {
	case nil:
		output.WriteString("null")
	case bool:
		output.WriteString(strconv.FormatBool(value))
	case json.Number:
		number, err := strconv.ParseFloat(string(value), 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return err
		}
		output.WriteString(ecmaNumber(number))
	case string:
		output.WriteString(approvalJSONString(value))
	case []any:
		output.WriteByte('[')
		for index, item := range value {
			if index > 0 {
				output.WriteByte(',')
			}
			if err := writeCanonicalApprovalJSON(output, item); err != nil {
				return err
			}
		}
		output.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
		output.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				output.WriteByte(',')
			}
			output.WriteString(approvalJSONString(key))
			output.WriteByte(':')
			if err := writeCanonicalApprovalJSON(output, value[key]); err != nil {
				return err
			}
		}
		output.WriteByte('}')
	default:
		return fmt.Errorf("unsupported JSON value %T", value)
	}
	return nil
}

func ecmaNumber(number float64) string {
	if math.IsInf(number, 0) || math.IsNaN(number) {
		return "null"
	}
	if number == 0 {
		return "0"
	}
	sign := ""
	if number < 0 {
		sign, number = "-", -number
	}
	format := byte('e')
	if number < 1e21 && number >= 1e-6 {
		format = 'f'
	}
	formatted := strconv.FormatFloat(number, format, -1, 64)
	if exponent := strings.IndexByte(formatted, 'e'); exponent > 0 &&
		exponent+2 < len(formatted) && formatted[exponent+2] == '0' {
		formatted = formatted[:exponent+2] + formatted[exponent+3:]
	}
	return sign + formatted
}

func utf16Less(left, right string) bool {
	l, r := utf16.Encode([]rune(left)), utf16.Encode([]rune(right))
	for index := 0; index < len(l) && index < len(r); index++ {
		if l[index] != r[index] {
			return l[index] < r[index]
		}
	}
	return len(l) < len(r)
}

func approvalJSONString(value string) string {
	var output strings.Builder
	output.WriteByte('"')
	for _, char := range value {
		switch char {
		case '"', '\\':
			output.WriteByte('\\')
			output.WriteRune(char)
		case '\b':
			output.WriteString(`\b`)
		case '\f':
			output.WriteString(`\f`)
		case '\n':
			output.WriteString(`\n`)
		case '\r':
			output.WriteString(`\r`)
		case '\t':
			output.WriteString(`\t`)
		default:
			if char < 0x20 {
				fmt.Fprintf(&output, `\u%04x`, char)
			} else {
				output.WriteRune(char)
			}
		}
	}
	output.WriteByte('"')
	return output.String()
}
