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

func (e *Errno) WithRaw(err error) *Errno {
	return &Errno{
		Type:    e.Type,
		Domain:  e.Domain,
		Code:    e.Code,
		Message: e.Message,
		RawErr:  err,
	}
}
