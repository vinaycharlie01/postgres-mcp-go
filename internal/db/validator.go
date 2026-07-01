package db

import (
	"fmt"
	"regexp"
	"strings"
)

// Validator enforces the strict read-only policy for all queries executed
// through the MCP server. It is intentionally conservative: anything that is
// not an obviously safe read is rejected.
//
// Validation properties:
//   - Case insensitive       (keywords matched regardless of case)
//   - Whitespace insensitive  (runs of whitespace collapsed before matching)
//   - Comment aware           (line and block comments stripped before matching)
//   - Multi-line aware        (newlines normalised, multiple statements detected)
type Validator struct {
	maxLength int
}

// NewValidator creates a Validator that rejects queries longer than maxLength
// characters.
func NewValidator(maxLength int) *Validator {
	if maxLength <= 0 {
		maxLength = 5000
	}
	return &Validator{maxLength: maxLength}
}

// allowedLeadingKeywords are the only statement keywords a query may begin with.
var allowedLeadingKeywords = []string{"SELECT", "WITH"}

// blockedKeywords are dangerous keywords that must never appear in a query.
// Each entry is matched on word boundaries (case-insensitively) against the
// comment-stripped, whitespace-normalised query.
var blockedKeywords = []string{
	// Data / schema modification
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE", "CREATE",
	"MERGE", "GRANT", "REVOKE", "DENY", "BACKUP", "RESTORE", "DBCC",
	// Execution
	"EXEC", "EXECUTE", "SP_EXECUTESQL", "XP_CMDSHELL", "SP_CONFIGURE",
	// Bulk / external data access
	"OPENROWSET", "OPENDATASOURCE", "OPENQUERY", "OPENXML",
	// Server / session control
	"SHUTDOWN", "ATTACH", "DETACH", "KILL", "RECONFIGURE",
	// Context / variables / flow control
	"USE", "SET", "DECLARE", "CURSOR", "WAITFOR",
	// Transaction control
	"COMMIT", "ROLLBACK",
}

// blockedPhrases are multi-word constructs that must be matched as a whole.
var blockedPhrases = []string{
	"BULK INSERT",
	"SELECT INTO",
	"BEGIN TRAN",
	"BEGIN TRANSACTION",
	"SAVE TRANSACTION",
}

// intoPattern detects an INTO clause (e.g. "SELECT ... INTO newtable"), which
// would create a table and is therefore forbidden.
var intoPattern = regexp.MustCompile(`(?i)\bINTO\b`)

// stmtSeparatorPattern detects statement separators that could be used to chain
// a second statement after an allowed SELECT.
var stmtSeparatorPattern = regexp.MustCompile(`;\s*\S`)

// stringLiteralPattern matches a T-SQL single-quoted string literal (with ''
// escaping). Literal contents are data, not executable SQL, so they are blanked
// before keyword matching to avoid false positives (e.g. WHERE name = 'INSERT').
var stringLiteralPattern = regexp.MustCompile(`'(?:[^']|'')*'`)

// Validate checks a query against the read-only policy. It returns nil when the
// query is safe to execute, or a descriptive error otherwise.
func (v *Validator) Validate(query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query must not be empty")
	}

	if len(query) > v.maxLength {
		return fmt.Errorf("query exceeds maximum length of %d characters (got %d)", v.maxLength, len(query))
	}

	// Normalise the query: strip comments, collapse whitespace.
	normalized := normalizeQuery(query)
	if normalized == "" {
		return fmt.Errorf("query contains no executable statement after removing comments")
	}

	upper := strings.ToUpper(normalized)

	// Must begin with an allowed keyword.
	if !hasAllowedPrefix(upper) {
		return fmt.Errorf("only SELECT and WITH (CTE) statements are allowed; query must start with SELECT or WITH")
	}

	// Reject chained statements (anything meaningful after a semicolon).
	if stmtSeparatorPattern.MatchString(strings.TrimRight(normalized, "; \t")) {
		return fmt.Errorf("multiple statements are not allowed; only a single read-only statement may be executed")
	}

	// Reject blocked multi-word phrases first (so "SELECT INTO" is caught
	// before the generic INTO check produces a less specific message).
	for _, phrase := range blockedPhrases {
		if containsPhrase(upper, phrase) {
			return fmt.Errorf("query contains forbidden construct %q; the server is strictly read-only", phrase)
		}
	}

	// Reject a bare INTO clause (SELECT ... INTO).
	if intoPattern.MatchString(upper) {
		return fmt.Errorf("query contains a forbidden INTO clause; the server is strictly read-only")
	}

	// Reject any blocked keyword on a word boundary.
	for _, keyword := range blockedKeywords {
		if containsWord(upper, keyword) {
			return fmt.Errorf("query contains forbidden keyword %q; the server is strictly read-only", keyword)
		}
	}

	return nil
}

// hasAllowedPrefix reports whether the upper-cased, normalised query starts with
// one of the allowed leading keywords. A leading open-paren is permitted so that
// queries such as "(SELECT ...) UNION (SELECT ...)" are accepted.
func hasAllowedPrefix(upper string) bool {
	trimmed := strings.TrimLeft(upper, "( \t")
	for _, kw := range allowedLeadingKeywords {
		if strings.HasPrefix(trimmed, kw+" ") || trimmed == kw || strings.HasPrefix(trimmed, kw+"(") {
			return true
		}
	}
	return false
}

// containsWord reports whether word appears in s on word boundaries. s is
// expected to be upper-cased already.
func containsWord(s, word string) bool {
	pattern := `\b` + regexp.QuoteMeta(word) + `\b`
	matched, _ := regexp.MatchString(pattern, s)
	return matched
}

// containsPhrase reports whether a multi-word phrase appears in s, tolerant of
// the amount of whitespace between the words. s is expected to be upper-cased.
func containsPhrase(s, phrase string) bool {
	parts := strings.Fields(phrase)
	for i := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	pattern := `\b` + strings.Join(parts, `\s+`) + `\b`
	matched, _ := regexp.MatchString(pattern, s)
	return matched
}

// normalizeQuery strips SQL comments and collapses runs of whitespace into a
// single space. It is comment-aware (both -- line comments and /* */ block
// comments) and multi-line aware.
func normalizeQuery(query string) string {
	stripped := stripComments(query)

	// Blank out string-literal contents so that keywords appearing inside data
	// values are not mistaken for executable statements.
	stripped = stringLiteralPattern.ReplaceAllString(stripped, "''")

	// Collapse all whitespace (including newlines and tabs) into single spaces.
	fields := strings.Fields(stripped)
	return strings.Join(fields, " ")
}

// stripComments removes T-SQL line comments (-- ... newline) and block comments
// (/* ... */) while preserving string literals. Nested block comments, which
// T-SQL supports, are handled by tracking nesting depth.
func stripComments(query string) string {
	var b strings.Builder
	runes := []rune(query)
	n := len(runes)

	inLineComment := false
	blockDepth := 0
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < n; i++ {
		c := runes[i]
		var next rune
		if i+1 < n {
			next = runes[i+1]
		}

		switch {
		case inLineComment:
			if c == '\n' {
				inLineComment = false
				b.WriteRune(c)
			}
		case blockDepth > 0:
			if c == '/' && next == '*' {
				blockDepth++
				i++
			} else if c == '*' && next == '/' {
				blockDepth--
				i++
			}
		case inSingleQuote:
			b.WriteRune(c)
			if c == '\'' {
				inSingleQuote = false
			}
		case inDoubleQuote:
			b.WriteRune(c)
			if c == '"' {
				inDoubleQuote = false
			}
		default:
			switch {
			case c == '-' && next == '-':
				inLineComment = true
				i++
			case c == '/' && next == '*':
				blockDepth++
				i++
			case c == '\'':
				inSingleQuote = true
				b.WriteRune(c)
			case c == '"':
				inDoubleQuote = true
				b.WriteRune(c)
			default:
				b.WriteRune(c)
			}
		}
	}

	return b.String()
}
