package api

import (
	"net/http"

	"arrowhead/core/internal/httputil"
)

// WriteErrorResponse writes an AH5-conformant ErrorResponse JSON body.
// If exType is empty, it is derived from status using httputil.ErrorTypeForStatus.
// This is a thin wrapper kept for backward-compatibility with existing tests.
func WriteErrorResponse(w http.ResponseWriter, status int, msg, exType, origin string) {
	if exType == "" {
		exType = httputil.ErrorTypeForStatus(status)
	}
	type errBody struct {
		ErrorMessage  string `json:"errorMessage"`
		ErrorCode     int    `json:"errorCode"`
		ExceptionType string `json:"exceptionType"`
		Origin        string `json:"origin"`
	}
	httputil.WriteJSON(w, status, errBody{
		ErrorMessage:  msg,
		ErrorCode:     status,
		ExceptionType: exType,
		Origin:        origin,
	}, origin)
}
