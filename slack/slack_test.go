package slack

import (
	"io/ioutil"
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

		for _, s := range []string{"foo", "#bar", "critical"} {
			if !strings.Contains(string(b), s) {
				t.Errorf("request expected to include %q", s)
			}
		}
	}))
	defer ts.Close()

	s, err := New(&Config{
		WebhookURL: ts.URL,
		Username:   "foo",
		Channel:    "#bar",
	})

	if err != nil {
		t.Fatal(err)
	}

	if err = s.Critical("app", "nginx"); err != nil {
		t.Fatal(err)
	}
}
