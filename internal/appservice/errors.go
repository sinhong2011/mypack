package appservice

import (
	"encoding/json"
	"errors"
	"fmt"

	"mypack.dev/mypack/internal/domain"
	"mypack.dev/mypack/pkg/config"
)

// Error codes produced by the service (in addition to the engine's).
const (
	CodeInvalidInput     = "INVALID_INPUT"
	CodeInvalidConfig    = "INVALID_CONFIG"
	CodeIdentityChange   = "IDENTITY_CHANGE_REQUIRES_CONFIRMATION"
	CodeRevisionConflict = "REVISION_CONFLICT"
	CodeAppRunning       = "APP_RUNNING"
	CodeNotManaged       = "NOT_MANAGED"
	CodeNotFound         = "NOT_FOUND"
	CodeDownloadRequired = "RUNTIME_DOWNLOAD_REQUIRED"
	CodeTargetUnsupp     = "TARGET_UNSUPPORTED"
	CodeCancelled        = "BUILD_CANCELLED"
	CodeBuildFailed      = "BUILD_FAILED"
	CodeInternal         = "INTERNAL"

	// Project build (running the user's JS build before packaging).
	CodeProjectBuildRequired = "PROJECT_BUILD_REQUIRED"
	CodeNodeNotFound         = "NODE_NOT_FOUND"
	CodeProjectBuildFailed   = "PROJECT_BUILD_FAILED"
	CodeProjectOutputMissing = "PROJECT_OUTPUT_MISSING"
	CodeApprovalRequired     = "APPROVAL_REQUIRED"
)

func newErr(code, msg, remediation string) *domain.Error {
	return &domain.Error{Code: code, Message: msg, Remediation: remediation}
}

// asDomainErr converts any error into a *domain.Error (code fallback if it
// is not one already).
func asDomainErr(err error, fallbackCode string) *domain.Error {
	if err == nil {
		return nil
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return de
	}
	var ve *config.ValidationError
	if errors.As(err, &ve) {
		return validationErr(ve)
	}
	return &domain.Error{Code: fallbackCode, Message: err.Error()}
}

// validationErr maps config validation to INVALID_CONFIG. The issues are
// carried as JSON in Cause; use ConfigIssues to decode them.
func validationErr(ve *config.ValidationError) *domain.Error {
	b, _ := json.Marshal(ve.Issues)
	msg := "The configuration is not valid."
	if len(ve.Issues) > 0 {
		msg = fmt.Sprintf("The configuration is not valid: %s: %s", ve.Issues[0].Field, ve.Issues[0].Message)
		if len(ve.Issues) > 1 {
			msg += fmt.Sprintf(" (and %d more)", len(ve.Issues)-1)
		}
	}
	return &domain.Error{
		Code:        CodeInvalidConfig,
		Message:     msg,
		Cause:       string(b),
		Remediation: "Fix the listed fields and try again.",
	}
}

// ConfigIssues extracts the field-level issues from an INVALID_CONFIG error.
func ConfigIssues(err error) []config.Issue {
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != CodeInvalidConfig {
		return nil
	}
	var is []config.Issue
	if json.Unmarshal([]byte(de.Cause), &is) != nil {
		return nil
	}
	return is
}
