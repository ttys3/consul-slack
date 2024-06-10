package discord

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

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
	)
	if err != nil {
		t.Fatal(err)
	}

	if err = s.Danger("bar"); err != nil {
		t.Fatal(err)
	}

	if !called {
		t.Fatal("http callback hasn't been called")
	}
}
