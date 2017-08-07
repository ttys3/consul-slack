package slack

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()

		for _, s := range []string{"foo", "#bar", "bar"} {
			if !strings.Contains(string(b), s) {
				t.Errorf("request expected to include %q", s)
			}
		}
	}))
	defer ts.Close()

	s, err := New(ts.URL,
		WithUsername("foo"),
		WithChannel("#bar"),
		WithLogger(log.New(ioutil.Discard, "", 0)),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err = s.Danger("bar"); err != nil {
		t.Fatal(err)
	}
}
