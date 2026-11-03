package db

import "strings"

// WithAction appends a SQLCommenter action tag. Statements that already
// carry a comment are left unchanged.
func WithAction(query, action string) string {
	if strings.Contains(query, "/*") {
		return query
	}
	return strings.TrimRight(query, " \t\r\n") + " /*action='" + action + "'*/"
}
