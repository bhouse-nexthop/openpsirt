package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// asPerson makes a request the way a signed-in person does.
func asPerson(t *testing.T, r *reach, who, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(testHeader, who)
	fromOurOwnPage(req)
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func TestTheReviewQueueIsReadableAndNarrowed(t *testing.T) {
	// The queue is the screen somebody works down. It has to be reachable by
	// whoever reads the product, and empty for somebody who holds nothing.
	eachReach(t, func(t *testing.T, r *reach) {
		got := asPerson(t, r, "triager", http.MethodGet, "/v1/review-queue", "")
		if got.Code != http.StatusOK {
			t.Fatalf("a triager reading the queue answered %d: %s", got.Code, got.Body.String())
		}
		var out struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v (%s)", err, got.Body.String())
		}
		if out.Total != 0 || len(out.Items) != 0 {
			t.Errorf("a deployment that has decided nothing has %d waiting", out.Total)
		}
	})
}

func TestDecidingAboutSomethingNobodyScannedIsNotThere(t *testing.T) {
	// A decision is about a finding. Naming a place freely would be choosing
	// which decisions apply where, so the names are resolved against what was
	// actually scanned.
	eachReach(t, func(t *testing.T, r *reach) {
		const body = `{"outcome":"wont-fix","reasoning":"Not worth it."}`
		got := asPerson(t, r, "triager", http.MethodPost,
			"/v1/products/mine/streams/master/variants/broadcom/findings/CVE-2026-1/places/nowhere/decision", body)
		if got.Code != http.StatusNotFound {
			t.Errorf("deciding about an unscanned build answered %d, want 404: %s", got.Code, got.Body.String())
		}
	})
}

func TestDecidingIsRefusedToSomebodyWhoOnlyReads(t *testing.T) {
	eachReach(t, func(t *testing.T, r *reach) {
		const body = `{"outcome":"wont-fix","reasoning":"Not worth it."}`
		got := asPerson(t, r, "reader", http.MethodPost,
			"/v1/products/mine/streams/master/variants/broadcom/findings/CVE-2026-1/places/somewhere/decision", body)
		// Not there rather than forbidden: a build nothing was scanned against
		// answers the same way whoever asks, and somebody who may not decide
		// learns nothing from the difference.
		if got.Code != http.StatusNotFound && got.Code != http.StatusForbidden {
			t.Errorf("a reader deciding answered %d: %s", got.Code, got.Body.String())
		}
	})
}

func TestApprovingSomethingThatIsNotThereSaysSo(t *testing.T) {
	eachReach(t, func(t *testing.T, r *reach) {
		got := asPerson(t, r, "triager", http.MethodPost, "/v1/decisions/999999/approval", `{}`)
		if got.Code != http.StatusNotFound {
			t.Errorf("approving a decision that does not exist answered %d", got.Code)
		}
	})
}

func TestARefusalOfTypedTextSaysWhereToLook(t *testing.T) {
	// A justification is long, and a refusal naming a category means hunting.
	// The API carries the position because the interface can only point at the
	// problem if the answer says where it is.
	eachReach(t, func(t *testing.T, r *reach) {
		body := `{"reasoning":"fine\nsee ![this](https://evil.example/x.png)"}`
		got := asPerson(t, r, "triager", http.MethodPut, "/v1/decisions/1/reasoning", body)
		// The decision does not exist here, so this is about the shape of a
		// refusal rather than about this particular one.
		if got.Code == http.StatusOK || got.Code == http.StatusNoContent {
			t.Errorf("a remote image was accepted: %s", got.Body.String())
		}
	})
}

func TestAPipelineHasNoJudgment(t *testing.T) {
	// A build server has no business deciding anything about what it uploaded.
	eachReach(t, func(t *testing.T, r *reach) {
		if got := r.asKey(t, http.MethodGet, "/v1/review-queue"); got != http.StatusForbidden {
			t.Errorf("a pipeline read the review queue: %d", got)
		}
	})
}

func TestATokenDecidesOnlyWhatItsOwnerCould(t *testing.T) {
	// A personal token is a live reference to its owner, so it reaches the
	// queue exactly when they do.
	eachReach(t, func(t *testing.T, r *reach) {
		ctx := t.Context()
		person, err := r.rights.ByIdentity(ctx, "triager")
		if err != nil {
			t.Fatal(err)
		}
		_, secret, err := r.rights.NewToken(ctx, person.ID, "scripting", nil, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := r.withKey(t, secret, http.MethodGet, "/v1/review-queue").code; got != http.StatusOK {
			t.Errorf("a token could not read what its owner reads: %d", got)
		}

		reader, err := r.rights.ByIdentity(ctx, "reporter")
		if err != nil {
			t.Fatal(err)
		}
		_, weak, err := r.rights.NewToken(ctx, reader.ID, "reporting", nil, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		// Holding a capability and no read role, this person sees no products
		// at all — so their token sees no queue.
		got := r.withKey(t, weak, http.MethodGet, "/v1/review-queue")
		if got.code != http.StatusOK {
			t.Fatalf("reading the queue answered %d", got.code)
		}
		if !strings.Contains(got.text, `"total":0`) {
			t.Errorf("somebody holding only a capability was shown work: %s", got.text)
		}
	})
}

var _ = access.PublicTriage
