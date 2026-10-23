package quota

import "testing"

func TestRecoveryHintDevinSignIn(t *testing.T) {
	got := RecoveryHint("devin", "Sign-in expired.")
	want := "Open Devin to renew its sign-in, then choose Refresh."
	if got != want {
		t.Fatalf("RecoveryHint = %q, want %q", got, want)
	}
}
