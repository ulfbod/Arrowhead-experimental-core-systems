package api

import "net/http"

// ErrorType constants match the AH5 ErrorResponse.exceptionType enum.
const (
	ErrTypeInvalidParameter    = "INVALID_PARAMETER"
	ErrTypeAuth                = "AUTH"
	ErrTypeForbidden           = "FORBIDDEN"
	ErrTypeDataNotFound        = "DATA_NOT_FOUND"
	ErrTypeLocked              = "LOCKED"
	ErrTypeInternalServerError = "INTERNAL_SERVER_ERROR"
)

// errorTypeForStatus maps HTTP status codes to AH5 exceptionType values.
func errorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusMethodNotAllowed:
		return ErrTypeInvalidParameter
	case http.StatusUnauthorized:
		return ErrTypeAuth
	case http.StatusForbidden:
		return ErrTypeForbidden
	case http.StatusNotFound:
		return ErrTypeDataNotFound
	case http.StatusLocked:
		return ErrTypeLocked
	default:
		return ErrTypeInternalServerError
	}
}

// WriteErrorResponse writes an AH5-conformant ErrorResponse JSON body.
// If exType is empty, it is derived from status using errorTypeForStatus.
func WriteErrorResponse(w http.ResponseWriter, status int, msg, exType, origin string) {
	if exType == "" {
		exType = errorTypeForStatus(status)
	}
	type errBody struct {
		ErrorMessage  string `json:"errorMessage"`
		ErrorCode     int    `json:"errorCode"`
		ExceptionType string `json:"exceptionType"`
		Origin        string `json:"origin"`
	}
	writeJSON(w, status, errBody{
		ErrorMessage:  msg,
		ErrorCode:     status,
		ExceptionType: exType,
		Origin:        origin,
	})
}
