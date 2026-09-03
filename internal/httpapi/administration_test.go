package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestGrantingARoleDoesNotAskWhetherItWorks holds the line that a grant is
// written with what the caller decides and read back with what the server
// knows.
//
// "effective" is the server's answer: an assignment set aside by a change of
// role-assignment mode is kept so the change can be undone, and it grants
// nothing while it sits there. Required on the way in, it made granting a role
// mean stating whether the role you are granting works — so everything that
// granted one sent "effective": true to be allowed to, and a caller that told
// the truth and left it out was refused with "expected required property
// effective to be present".
func TestGrantingARoleDoesNotAskWhetherItWorks(t *testing.T) {
	twoReach(t, func(t *testing.T, r *reach) {
		recorded := asPerson(t, r, "admin", http.MethodPost, "/v1/people",
			`{"identity":"ana","display_name":"Ana","provider":"proxy","username":"ana",`+
				`"holds":[{"product":"mine","role":"public-read"}]}`)
		if recorded.Code != http.StatusCreated {
			t.Fatalf("recording somebody with a role answered %d: %s",
				recorded.Code, recorded.Body.String())
		}

		// And the reply describes the record. It used to be the request handed
		// back, so whatever the caller said about a grant came back as though
		// the deployment had confirmed it.
		var made struct {
			Item struct {
				Holds []struct {
					Effective bool   `json:"effective"`
					Source    string `json:"source"`
				} `json:"holds"`
				SignsInBy []struct {
					Provider string `json:"provider"`
				} `json:"signs_in_by"`
			} `json:"item"`
		}
		if err := json.Unmarshal(recorded.Body.Bytes(), &made); err != nil {
			t.Fatalf("decode: %v (%s)", err, recorded.Body.String())
		}
		if len(made.Item.Holds) != 1 {
			t.Fatalf("recording answered with %d roles, not the one granted: %s",
				len(made.Item.Holds), recorded.Body.String())
		}
		if !made.Item.Holds[0].Effective || made.Item.Holds[0].Source != "assigned" {
			t.Errorf("the reply describes the grant as %+v, which is not what was recorded",
				made.Item.Holds[0])
		}
		if len(made.Item.SignsInBy) != 1 || made.Item.SignsInBy[0].Provider != "proxy" {
			t.Errorf("the reply does not say how she can arrive: %s", recorded.Body.String())
		}

		// And the role is a real one, not a body that merely parsed: she can
		// reach the product it was held against, and nothing else.
		if got := r.as(t, "ana", http.MethodGet, "/v1/products/mine/streams"); got != http.StatusOK {
			t.Fatalf("the person just granted a role on mine reads it as %d", got)
		}
		if got := r.as(t, "ana", http.MethodGet, "/v1/products/theirs/streams"); got != http.StatusNotFound {
			t.Fatalf("she reads a product she holds nothing on as %d, not 404", got)
		}

		// Read back, the answer the request did not have to state is there.
		listed := asPerson(t, r, "admin", http.MethodGet, "/v1/people", "")
		if listed.Code != http.StatusOK {
			t.Fatalf("listing people answered %d: %s", listed.Code, listed.Body.String())
		}
		var out struct {
			Items []struct {
				Identity string `json:"identity"`
				Holds    []struct {
					Product   string `json:"product"`
					Role      string `json:"role"`
					Effective bool   `json:"effective"`
					Source    string `json:"source"`
				} `json:"holds"`
			} `json:"items"`
		}
		if err := json.Unmarshal(listed.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v (%s)", err, listed.Body.String())
		}
		var found bool
		for _, person := range out.Items {
			if person.Identity != "ana" {
				continue
			}
			for _, hold := range person.Holds {
				if hold.Role != "public-read" {
					continue
				}
				found = true
				if !hold.Effective {
					t.Errorf("the role she was granted is read back as granting nothing")
				}
				if hold.Source != "assigned" {
					t.Errorf("a role an administrator granted came back sourced %q", hold.Source)
				}
			}
		}
		if !found {
			t.Fatalf("the role recorded with her is not read back: %s", listed.Body.String())
		}
	})
}
