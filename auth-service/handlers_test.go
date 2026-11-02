package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChaosHandlerFazValidateResponder500(t *testing.T) {
	app := &App{}

	post := httptest.NewRecorder()
	app.chaosHandler(post, httptest.NewRequest(http.MethodPost, "/admin/chaos", nil))
	if post.Code != http.StatusNoContent {
		t.Fatalf("POST /admin/chaos: esperado 204, recebido %d", post.Code)
	}

	validate := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	req.Header.Set("Authorization", "Bearer qualquer")
	app.validateKeyHandler(validate, req)
	if validate.Code != http.StatusInternalServerError {
		t.Fatalf("/validate com falha ativa: esperado 500, recebido %d", validate.Code)
	}

	app.chaosHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/admin/chaos", nil))
	if app.faultInjected.Load() {
		t.Fatal("DELETE /admin/chaos deveria desligar a falha")
	}
}

func TestChaosHandlerRecusaOutrosMetodos(t *testing.T) {
	rec := httptest.NewRecorder()
	(&App{}).chaosHandler(rec, httptest.NewRequest(http.MethodGet, "/admin/chaos", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, recebido %d", rec.Code)
	}
}
