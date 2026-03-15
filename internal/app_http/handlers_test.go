package apphttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultHandler(t *testing.T) {
	mux := http.NewServeMux()
	defaultHandler := NewDefaultHandler()
	defaultHandler.RegisterRoutes(mux)
	req, err := http.NewRequest("GET", "/", nil)

	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
	})
	
	handler.ServeHTTP(recorder, req)
	
	if string(recorder.Body.String()) != "Hello 2313121" {
		t.Errorf("expected 'Hello 2313121', got %s", recorder.Body.String())
	}
}
