package analysis

import (
	"fmt"
	"strings"
)

// ValidateSig validates the contents inside :SIG(...).
// It returns nil if the signature is syntactically valid.
func ValidateSig(sig string) error {
	s := strings.TrimSpace(sig)
	if s == "" {
		return fmt.Errorf("empty signature")
	}
	if left, right, ok := splitTopLevelArrow(s); ok {
		if err := validateArgList(left); err != nil {
			return fmt.Errorf("invalid args: %w", err)
		}
		if err := validateRetList(right); err != nil {
			return fmt.Errorf("invalid return: %w", err)
		}
		return nil
	}
	if err := validateType(s, false); err != nil {
		return err
	}
	return nil
}

// ParseSigArgs returns argument types for a function signature.
// For "void" it returns an empty slice.
func ParseSigArgs(sig string) ([]string, error) {
	s := strings.TrimSpace(sig)
	if s == "" {
		return nil, fmt.Errorf("empty signature")
	}
	left, _, ok := splitTopLevelArrow(s)
	if !ok {
		return nil, fmt.Errorf("not a function signature")
	}
	return parseTypeList(left, true)
}

// ParseSigReturn returns return types for a function signature.
// For "void" it returns an empty slice.
func ParseSigReturn(sig string) ([]string, error) {
	s := strings.TrimSpace(sig)
	if s == "" {
		return nil, fmt.Errorf("empty signature")
	}
	_, right, ok := splitTopLevelArrow(s)
	if !ok {
		return nil, fmt.Errorf("not a function signature")
	}
	return parseTypeList(right, true)
}

func splitTopLevelArrow(s string) (string, string, bool) {
	depthParen := 0
	depthBracket := 0
	for i := 0; i+1 < len(s); i++ {
		ch := s[i]
		switch ch {
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case '-':
			if s[i+1] == '>' && depthParen == 0 && depthBracket == 0 {
				left := strings.TrimSpace(s[:i])
				right := strings.TrimSpace(s[i+2:])
				if left == "" || right == "" {
					return "", "", false
				}
				return left, right, true
			}
		}
	}
	return "", "", false
}

func validateArgList(s string) error {
	_, err := parseTypeList(s, true)
	return err
}

func validateRetList(s string) error {
	_, err := parseTypeList(s, true)
	return err
}

func parseTypeList(s string, allowVoid bool) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty list")
	}
	if s == "void" || s == "(void)" {
		if !allowVoid {
			return nil, fmt.Errorf("void not allowed")
		}
		return []string{}, nil
	}
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		body := strings.TrimSpace(s[1 : len(s)-1])
		if body == "" {
			return nil, fmt.Errorf("empty list")
		}
		parts := splitTopLevel(body, ',')
		if len(parts) < 2 {
			item := strings.TrimSpace(body)
			if err := validateType(item, allowVoid); err != nil {
				return nil, err
			}
			return []string{item}, nil
		}
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				return nil, fmt.Errorf("empty type")
			}
			if err := validateType(part, allowVoid); err != nil {
				return nil, err
			}
			out = append(out, part)
		}
		return out, nil
	}
	if strings.Contains(s, ",") {
		return nil, fmt.Errorf("multiple types require parentheses")
	}
	if err := validateType(s, allowVoid); err != nil {
		return nil, err
	}
	return []string{s}, nil
}

func validateType(s string, allowVoid bool) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("empty type")
	}
	switch s {
	case "any", "int", "undef":
		return nil
	case "void":
		if allowVoid {
			return nil
		}
		return fmt.Errorf("void not allowed here")
	}
	if strings.HasPrefix(s, "array[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSpace(s[len("array[") : len(s)-1])
		if inner == "" {
			return fmt.Errorf("array[] missing type")
		}
		return validateType(inner, allowVoid)
	}
	if strings.HasPrefix(s, "hash[") && strings.HasSuffix(s, "]") {
		inner := strings.TrimSpace(s[len("hash[") : len(s)-1])
		if inner == "" {
			return fmt.Errorf("hash[] missing type")
		}
		return validateType(inner, allowVoid)
	}
	if isClassName(s) {
		return nil
	}
	return fmt.Errorf("unknown type %q", s)
}

func splitTopLevel(s string, sep byte) []string {
	var out []string
	depthParen := 0
	depthBracket := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case sep:
			if depthParen == 0 && depthBracket == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

func isClassName(s string) bool {
	if s == "" {
		return false
	}
	for part := range strings.SplitSeq(s, "::") {
		if part == "" {
			return false
		}
		if !isIdent(part) {
			return false
		}
	}
	return true
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if i == 0 {
			if !(ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')) {
				return false
			}
			continue
		}
		if !(ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
			return false
		}
	}
	return true
}
