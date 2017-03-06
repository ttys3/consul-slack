package consul

import (
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
)

func TestConsul(t *testing.T) {
	cmd := exec.Command("consul", "agent", "-dev")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	go func() {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	cc, err := api.NewClient(&api.Config{})
	if err != nil {
		t.Fatal(err)
	}

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	port := lis.Addr().(*net.TCPAddr).Port
	err = cc.Agent().ServiceRegister(&api.AgentServiceRegistration{
		Name:    "foo",
		Port:    port,
		Address: "::1",
		Checks: api.AgentServiceChecks{
			{
				Notes:    "HTTP Check",
				HTTP:     "http://localhost:" + strconv.Itoa(port),
				Interval: "1s",
				Timeout:  "1s",
			},
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	c, err := New(&Config{})
	if err != nil {
		t.Fatal(err)
	}

	testNext(t, "fail", 2*time.Second, c, 1, 0)

	// start service
	go func() {
		m := http.NewServeMux()
		m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		s := http.Server{Handler: m}
		s.Serve(lis)
	}()

	testNext(t, "pass", 2*time.Second, c, 0, 1)
	testNext(t, "gone", time.Duration(0), c, 0, 0)
}

func testNext(t *testing.T, name string, delay time.Duration, c *Consul, ccl, pcl int) {
	time.Sleep(delay)

	cc, pc, err := c.Next()
	if err != nil {
		t.Fatal(err)
	}

	if len(cc) != ccl {
		t.Errorf("%s: len(cc) = %d, want %d", name, len(cc), ccl)
	}
	if len(pc) != pcl {
		t.Errorf("%s: len(pc) = %d, want %d", name, len(pc), pcl)
	}
}
