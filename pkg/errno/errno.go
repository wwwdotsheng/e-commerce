package errno

import "fmt"

type Errno struct {
	Type    string
	Domain  string
	Code    string
	Message string
	RawErr  error
}

func (e *Errno) FullCode() string {
	return fmt.Sprintf("%s%s%s", e.Type, e.Domain, e.Code)
}

func (e *Errno) Error() string {
	if e.RawErr != nil {
		return fmt.Sprintf("Code: %s, Msg: %s, Raw: %v", e.FullCode(), e.Message, e.RawErr)
	}
	return fmt.Sprintf("Code: %s, Msg: %s", e.FullCode(), e.Message)
}

func (e *Errno) Unwrap() error {
	return e.RawErr
}

// Is 按错误码匹配，保证 WithRaw 复制出来的 Errno 仍能被 errors.Is 识别。
func (e *Errno) Is(target error) bool {
	t, ok := target.(*Errno)
	return ok && t.FullCode() == e.FullCode()
}

func (e *Errno) WithRaw(err error) *Errno {
	return &Errno{
		Type:    e.Type,
		Domain:  e.Domain,
		Code:    e.Code,
		Message: e.Message,
		RawErr:  err,
	}
}
