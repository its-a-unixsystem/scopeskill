package scopeskill

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchKontaktByIDProjectsMasterFields(t *testing.T) {
	var requested string
	var fields string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		fields = r.URL.Query().Get("fields")
		writeJSON(w, map[string]any{
			"id":             49189,
			"lastname":       "Ingenire UG (haftungsbeschränkt)",
			"vatId":          "DE366054310",
			"debitorNumber":  nil,
			"kreditorNumber": nil,
		})
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
	got, err := FetchKontaktByID(client, 49189)
	if err != nil {
		t.Fatal(err)
	}
	if got["lastname"] != "Ingenire UG (haftungsbeschränkt)" {
		t.Fatalf("got = %#v", got)
	}
	if requested != "/rest/contact/49189" {
		t.Fatalf("path = %q", requested)
	}
	for _, want := range []string{"id", "lastname", "companyname", "vatId", "debitorNumber", "kreditorNumber"} {
		if !strings.Contains(fields, want) {
			t.Fatalf("fields missing %q in %q", want, fields)
		}
	}
}

func TestFetchKontaktByIDReturnsNilOn400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such contact", http.StatusBadRequest)
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
	got, err := FetchKontaktByID(client, 999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("got = %#v", got)
	}
}

func TestFetchKontaktByIDReturnsNilOn404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
	got, err := FetchKontaktByID(client, 999)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("got = %#v", got)
	}
}

func TestFetchKontaktByIDPropagatesOtherErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
	if _, err := FetchKontaktByID(client, 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchKontaktByIDRejectsEmptyID(t *testing.T) {
	if _, err := FetchKontaktByID(nil, ""); err == nil {
		t.Fatal("expected error")
	}
	if _, err := FetchKontaktByID(nil, nil); err == nil {
		t.Fatal("expected error")
	}
}
