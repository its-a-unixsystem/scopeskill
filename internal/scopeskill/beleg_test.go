package scopeskill

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchBelegGetsEndpointNumber(t *testing.T) {
	var requested string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		writeJSON(w, map[string]any{"number": "2026-1", "documentNumber": "2HUJ9MS4-0002"})
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
	got, err := FetchBeleg(client, BelegEndpointIncomingInvoice, "2026-1")
	if err != nil {
		t.Fatal(err)
	}
	if requested != "/rest/incominginvoice/2026-1" {
		t.Fatalf("path = %q", requested)
	}
	if got["documentNumber"] != "2HUJ9MS4-0002" {
		t.Fatalf("got = %#v", got)
	}
}

func TestFetchBelegReturnsNilOn400And404(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "missing", status)
			}))
			defer server.Close()
			client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
			got, err := FetchBeleg(client, BelegEndpointIncomingInvoice, "missing")
			if err != nil {
				t.Fatal(err)
			}
			if got != nil {
				t.Fatalf("got = %#v", got)
			}
		})
	}
}

func TestFetchBelegRejectsMissingInputs(t *testing.T) {
	if _, err := FetchBeleg(nil, "", "1"); err == nil {
		t.Fatal("expected endpoint error")
	}
	if _, err := FetchBeleg(nil, BelegEndpointIncomingInvoice, ""); err == nil {
		t.Fatal("expected number error")
	}
}
