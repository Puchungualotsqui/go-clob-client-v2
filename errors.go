package clob

import (
	"fmt"
	"net/http"
)

type ErrorKind string

const (
	KindStatus          ErrorKind = "Status"
	KindValidation      ErrorKind = "Validation"
	KindSynchronization ErrorKind = "Synchronization"
	KindInternal        ErrorKind = "Internal"
	KindWebSocket       ErrorKind = "WebSocket"
	KindGeoblock        ErrorKind = "Geoblock"
)

type Error struct {
	Kind   ErrorKind
	Source error
}

func (e *Error) Error() string {
	if e.Source == nil {
		return string(e.Kind)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Source)
}
func (e *Error) Unwrap() error                { return e.Source }
func NewValidationError(reason string) *Error { return &Error{KindValidation, ValidationError{reason}} }
func NewStatusError(code int, method, path, message string) *Error {
	return &Error{KindStatus, StatusError{code, method, path, message}}
}
func NewMissingContractConfigError(chain uint64, negRisk bool) *Error {
	return &Error{KindInternal, MissingContractConfigError{chain, negRisk}}
}

type ValidationError struct{ Reason string }

func (e ValidationError) Error() string { return "invalid: " + e.Reason }

type StatusError struct {
	StatusCode            int
	Method, Path, Message string
}

func (e StatusError) Error() string {
	status := fmt.Sprint(e.StatusCode)
	if text := http.StatusText(e.StatusCode); text != "" {
		status += " " + text
	}
	return fmt.Sprintf("error(%s) making %s call to %s with %s", status, e.Method, e.Path, e.Message)
}

type SynchronizationError struct{}

func (SynchronizationError) Error() string {
	return "synchronization error: multiple threads are attempting to log in or log out"
}

type MissingContractConfigError struct {
	ChainID uint64
	NegRisk bool
}

func (e MissingContractConfigError) Error() string {
	return fmt.Sprintf("missing contract config for chain id %d with neg_risk = %t", e.ChainID, e.NegRisk)
}

type GeoblockError struct{ IP, Country, Region string }

func (e GeoblockError) Error() string {
	return fmt.Sprintf("access blocked from country: %s, region: %s, ip: %s", e.Country, e.Region, e.IP)
}
