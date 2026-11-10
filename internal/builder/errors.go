package builder

import "mypack.dev/mypack/internal/domain"

// domainErr is a lightweight carrier converted to *domain.Error by wrap.
type domainErr struct {
	code, field, msg, rem string
	cause                 error
}

func (e *domainErr) Error() string { return e.code + ": " + e.msg }

func (e *domainErr) toDomain() *domain.Error {
	d := &domain.Error{Code: e.code, Message: e.msg, Field: e.field, Remediation: e.rem}
	if e.cause != nil {
		d.Cause = e.cause.Error()
	}
	return d
}
