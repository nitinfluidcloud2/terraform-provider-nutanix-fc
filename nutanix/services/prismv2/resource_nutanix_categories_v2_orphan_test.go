package prismv2

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	prismapi "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/api"
	prismclient "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/client"
	prismsdk "github.com/terraform-providers/terraform-provider-nutanix/nutanix/sdks/v4/prism"
)

// fakeNutanix spins up an httptest TLS server that mimics the subset of the
// Prism Central v4.0.a1 categories API we exercise in the orphan-parent
// helpers. The caller supplies routes keyed by "<METHOD> <path>". Path
// matching ignores query string but path includes the version prefix.
type fakeNutanix struct {
	server *httptest.Server
	// hits[i] is the i-th request, captured for assertions.
	hits []recordedRequest
}

type recordedRequest struct {
	method string
	path   string
	query  url.Values
}

type fakeRoute struct {
	status int
	body   string
}

func newFakeNutanix(t *testing.T, routes map[string]fakeRoute) *fakeNutanix {
	t.Helper()
	fn := &fakeNutanix{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fn.hits = append(fn.hits, recordedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
		})
		key := r.Method + " " + r.URL.Path
		if route, ok := routes[key]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(route.status)
			_, _ = w.Write([]byte(route.body))
			return
		}
		// default 404 so unexpected routes fail loud
		http.Error(w, "no route configured for "+key, http.StatusNotFound)
	})
	fn.server = httptest.NewTLSServer(mux)
	t.Cleanup(fn.server.Close)
	return fn
}

// client returns a *prismsdk.Client whose CategoriesAPIInstance.ApiClient
// points at the fake server. Mirrors the structure built by
// prismsdk.NewPrismClient but with no version negotiation and no real auth.
func (fn *fakeNutanix) client(t *testing.T) *prismsdk.Client {
	t.Helper()
	u, err := url.Parse(fn.server.URL)
	if err != nil {
		t.Fatalf("parse httptest URL %q: %v", fn.server.URL, err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse httptest port %q: %v", u.Port(), err)
	}
	ac := prismclient.NewApiClient()
	ac.Host = u.Hostname()
	ac.Port = port
	ac.Username = "test-user"
	ac.Password = "test-pass"
	ac.VerifySSL = false // matches httptest.NewTLSServer's self-signed cert
	// Disable version negotiation so the SDK does not try to OPTIONS the fake.
	ac.AllowVersionNegotiation = false
	return &prismsdk.Client{
		CategoriesAPIInstance: prismapi.NewCategoriesApi(ac),
	}
}

// --- discoverParentBucketExtID ------------------------------------------------

func TestDiscoverParentBucketExtID_Success(t *testing.T) {
	body := `{
		"data": [
			{
				"extId": "11111111-2222-3333-4444-555555555555",
				"name": "fc_orphan_smoke",
				"parentExtId": null,
				"type": "USER"
			}
		]
	}`
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /api/prism/v4.0.a1/config/categories": {status: 200, body: body},
	})
	got, err := discoverParentBucketExtID(fn.client(t), "fc_orphan_smoke")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("got parent extId %q, want 11111111-...", got)
	}
	// Verify we hit v4.0.a1 (not v4.2 / v4.3 — the regression we are guarding against)
	if len(fn.hits) != 1 {
		t.Fatalf("expected 1 request, got %d", len(fn.hits))
	}
	if !strings.Contains(fn.hits[0].path, "/v4.0.a1/") {
		t.Fatalf("expected v4.0.a1 in path, got %q", fn.hits[0].path)
	}
	// And that the filter was passed correctly
	got1 := fn.hits[0].query.Get("$filter")
	want1 := "name eq 'fc_orphan_smoke' and parentExtId eq null"
	if got1 != want1 {
		t.Fatalf("filter mismatch:\n got=%q\nwant=%q", got1, want1)
	}
}

func TestDiscoverParentBucketExtID_EmptyKey(t *testing.T) {
	// No fake server — empty key should short-circuit without an HTTP call.
	fn := newFakeNutanix(t, nil)
	got, err := discoverParentBucketExtID(fn.client(t), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty parent for empty key, got %q", got)
	}
	if len(fn.hits) != 0 {
		t.Fatalf("expected 0 requests for empty key, got %d", len(fn.hits))
	}
}

func TestDiscoverParentBucketExtID_NoMatch(t *testing.T) {
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /api/prism/v4.0.a1/config/categories": {status: 200, body: `{"data":[]}`},
	})
	got, err := discoverParentBucketExtID(fn.client(t), "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty result for no match, got %q", got)
	}
}

func TestDiscoverParentBucketExtID_SkipsChildEntries(t *testing.T) {
	// Defensive belt+braces: even if the server returns a row with a non-nil
	// parentExtId (which would mean it is NOT a root bucket), we must not
	// pick it up. The $filter pins parentExtId eq null server-side already;
	// this protects against a buggy server.
	body := `{
		"data": [
			{
				"extId": "child-extid",
				"name": "x",
				"parentExtId": "real-parent",
				"type": "USER"
			}
		]
	}`
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /api/prism/v4.0.a1/config/categories": {status: 200, body: body},
	})
	got, err := discoverParentBucketExtID(fn.client(t), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected to skip child entry, got %q", got)
	}
}

func TestDiscoverParentBucketExtID_HTTPError(t *testing.T) {
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /api/prism/v4.0.a1/config/categories": {status: 500, body: `{"error":"boom"}`},
	})
	got, err := discoverParentBucketExtID(fn.client(t), "anything")
	if err == nil {
		t.Fatalf("expected error on HTTP 500, got nil (parent=%q)", got)
	}
	if got != "" {
		t.Fatalf("expected empty parent on error, got %q", got)
	}
}

func TestDiscoverParentBucketExtID_GarbledBody(t *testing.T) {
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /api/prism/v4.0.a1/config/categories": {status: 200, body: `not json`},
	})
	_, err := discoverParentBucketExtID(fn.client(t), "anything")
	if err == nil {
		t.Fatalf("expected parse error on garbled body")
	}
}

// --- isParentBucketEmpty ------------------------------------------------------

func TestIsParentBucketEmpty_True(t *testing.T) {
	parentID := "aaaa-bbbb-cccc"
	body := fmt.Sprintf(`{
		"data": {
			"extId": "%s",
			"name": "fc_orphan_smoke",
			"parentExtId": null,
			"childCategories": []
		}
	}`, parentID)
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /api/prism/v4.0.a1/config/categories/" + parentID: {status: 200, body: body},
	})
	got, err := isParentBucketEmpty(fn.client(t), parentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("expected parent to be reported empty")
	}
	// Confirm we requested $expand=childCategories — without that the server
	// would not return the array at all and the result would be ambiguous.
	if got := fn.hits[0].query.Get("$expand"); got != "childCategories" {
		t.Fatalf("expand mismatch: got=%q want=childCategories", got)
	}
}

func TestIsParentBucketEmpty_HasChildren(t *testing.T) {
	parentID := "aaaa-bbbb-cccc"
	body := fmt.Sprintf(`{
		"data": {
			"extId": "%s",
			"name": "shared_key",
			"parentExtId": null,
			"childCategories": [
				{"extId": "child-1", "name": "sibling1", "parentExtId": "%s"},
				{"extId": "child-2", "name": "sibling2", "parentExtId": "%s"}
			]
		}
	}`, parentID, parentID, parentID)
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /api/prism/v4.0.a1/config/categories/" + parentID: {status: 200, body: body},
	})
	got, err := isParentBucketEmpty(fn.client(t), parentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Fatalf("expected parent NOT to be reported empty (2 children present)")
	}
}

func TestIsParentBucketEmpty_404TreatedAsEmpty(t *testing.T) {
	// Idempotency requirement: if the parent bucket has already been removed
	// (e.g. cluster auto-cleaned it, or a prior destroy raced ahead), the
	// destroy path should still succeed.
	parentID := "aaaa-bbbb-cccc"
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /api/prism/v4.0.a1/config/categories/" + parentID: {status: 404, body: `{"error":"not found"}`},
	})
	got, err := isParentBucketEmpty(fn.client(t), parentID)
	if err != nil {
		t.Fatalf("expected nil error on 404, got %v", err)
	}
	if !got {
		t.Fatalf("expected 404 to be treated as empty=true")
	}
}

func TestIsParentBucketEmpty_EmptyParentExt(t *testing.T) {
	fn := newFakeNutanix(t, nil)
	_, err := isParentBucketEmpty(fn.client(t), "")
	if err == nil {
		t.Fatalf("expected error for empty parentExtID")
	}
}

// --- callPrismRaw -------------------------------------------------------------

func TestCallPrismRaw_404IsSuccessWhenAccepted(t *testing.T) {
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /any/path": {status: 404, body: `{"error":"gone"}`},
	})
	body, err := callPrismRaw(fn.client(t), "GET", "/any/path", true)
	if err != nil {
		t.Fatalf("acceptNotFound=true: 404 should be success, got err=%v", err)
	}
	if !strings.Contains(string(body), "gone") {
		t.Fatalf("expected body to be returned on 404, got %q", string(body))
	}
}

func TestCallPrismRaw_404IsErrorWhenNotAccepted(t *testing.T) {
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"DELETE /any/path": {status: 404, body: `{"error":"gone"}`},
	})
	body, err := callPrismRaw(fn.client(t), "DELETE", "/any/path", false)
	if err == nil {
		t.Fatalf("acceptNotFound=false: 404 should be error, got nil")
	}
	// Body is still returned so the caller can log it.
	if !strings.Contains(string(body), "gone") {
		t.Fatalf("expected body to be returned on 404 error, got %q", string(body))
	}
}

func TestCallPrismRaw_204IsAlwaysSuccess(t *testing.T) {
	// The actual happy path for our v4.0.a1 DELETE on cluster-1919.
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"DELETE /api/prism/v4.0.a1/config/categories/some-parent-id": {status: 204, body: ""},
	})
	for _, accept := range []bool{true, false} {
		_, err := callPrismRaw(fn.client(t), "DELETE", "/api/prism/v4.0.a1/config/categories/some-parent-id", accept)
		if err != nil {
			t.Fatalf("acceptNotFound=%v: 204 should be success, got %v", accept, err)
		}
	}
}

func TestCallPrismRaw_500IsAlwaysError(t *testing.T) {
	fn := newFakeNutanix(t, map[string]fakeRoute{
		"GET /any/path": {status: 500, body: `boom`},
	})
	for _, accept := range []bool{true, false} {
		_, err := callPrismRaw(fn.client(t), "GET", "/any/path", accept)
		if err == nil {
			t.Fatalf("acceptNotFound=%v: 500 should be error, got nil", accept)
		}
	}
}

func TestCallPrismRaw_NilClient(t *testing.T) {
	_, err := callPrismRaw(nil, "GET", "/anything", false)
	if err == nil {
		t.Fatalf("expected error for nil client")
	}
}

func TestCallPrismRaw_EmptyHost(t *testing.T) {
	// Client structurally valid but Host empty — should refuse to call.
	ac := prismclient.NewApiClient()
	ac.Host = ""
	conn := &prismsdk.Client{CategoriesAPIInstance: prismapi.NewCategoriesApi(ac)}
	_, err := callPrismRaw(conn, "GET", "/anything", false)
	if err == nil {
		t.Fatalf("expected error for empty host")
	}
}
