package quota

import "strings"

// RecoveryHint is the one-line fix suggestion under a failed reading. It lives
// in quota so both the standalone widget and the service protocol send the
// same words.
func RecoveryHint(id, reason string) string {
	lower := strings.ToLower(reason)
	if strings.Contains(lower, "sign") || strings.Contains(lower, "login") || strings.Contains(lower, "auth") {
		tool := ItemShort(id)
		switch id {
		case ItemCopilot:
			return "Run gh auth login, then choose Refresh."
		case ItemKiro:
			return "Run kiro-cli login, then choose Refresh."
		case ItemClaude:
			tool = "Claude Code"
		case ItemAntigravity:
			tool = "agy"
		}
		return "Open " + tool + " to renew its sign-in, then choose Refresh."
	}
	return "Check " + ItemShort(id) + " and its connection, then choose Refresh. Usage checks retry automatically."
}
