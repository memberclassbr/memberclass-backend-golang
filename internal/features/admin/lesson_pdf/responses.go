package lesson_pdf

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
)

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError uses the {ok, error, errorCode} shape these endpoints have always
// returned. Every failure here carries INTERNAL_ERROR — the admin UI shows the
// message rather than switching on the code.
func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": "INTERNAL_ERROR",
	})
}

// writeUseCaseError maps a pipeline failure to its HTTP status, keeping the
// domain message and hiding anything unexpected behind a generic 500.
func (f *Feature) writeUseCaseError(w http.ResponseWriter, err error) {
	var mcErr *memberclasserrors.MemberClassError
	if errors.As(err, &mcErr) {
		writeError(w, mcErr.Code, mcErr.Message)
		return
	}
	f.log.Error("Unexpected error: " + err.Error())
	writeError(w, http.StatusInternalServerError, "Erro interno do servidor")
}
