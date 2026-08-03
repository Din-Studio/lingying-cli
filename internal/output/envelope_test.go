package output

import (
	"errors"
	"testing"
)

func TestJSONFailureReturnsEmittedError(t *testing.T) {
	err := JSON(Failure("insufficient_balance", "余额不足", nil))
	if !IsEmittedError(err) {
		t.Fatalf("JSON() error = %v, want emitted error", err)
	}
	if !errors.Is(err, ErrEmitted) {
		t.Fatalf("JSON() error = %v, want ErrEmitted", err)
	}
}
