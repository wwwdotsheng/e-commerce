package errno

import (
	"errors"
	"testing"
)

func TestErrnoWithRawKeepsErrorChain(t *testing.T) {
	cause := errors.New("root cause")
	wrapped := ErrInvalidParam.WithRaw(cause)

	if !errors.Is(wrapped, ErrInvalidParam) {
		t.Fatal("WithRaw 结果应能匹配原错误码")
	}
	if !errors.Is(wrapped, cause) {
		t.Fatal("WithRaw 应能通过 Unwrap 找到根因")
	}
	if got := errors.Unwrap(wrapped); got != cause {
		t.Fatalf("Unwrap() = %v, want %v", got, cause)
	}
}

func TestErrnoIsComparesCode(t *testing.T) {
	other := &Errno{Type: ErrInvalidParam.Type, Domain: ErrInvalidParam.Domain, Code: ErrInvalidParam.Code, Message: "other message"}
	if !errors.Is(other, ErrInvalidParam) {
		t.Fatal("相同错误码应匹配，即使指针和文案不同")
	}
	if errors.Is(ErrNotFoundRecord, ErrInvalidParam) {
		t.Fatal("不同错误码不应匹配")
	}
}
