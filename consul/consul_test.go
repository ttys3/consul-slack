package consul

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
)

func TestConsul_Next(t *testing.T) {
	p := startConsul(t)
	defer stopConsul(t, p)

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

	c1, err := New(&Config{})
	if err != nil {
		t.Fatal(err)
	}

	testNext(t, "fail", 2*time.Second, c1, 1, 0)

	// start service
	go func() {
		m := http.NewServeMux()
		m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		s := http.Server{Handler: m}
		s.Serve(lis)
	}()

	ch := make(chan struct{})
	go func() {
		c2, err := New(&Config{})
		if err != nil {
			t.Fatal(err)
		}

		testNext(t, "pass", 2*time.Second, c2, 0, 1)

		go func() {
			if err := c2.Close(); err != nil {
				t.Fatal(err)
			}
		}()

		testNext(t, "gone", time.Duration(0), c2, 0, 0)
		close(ch)
	}()

	if err = c1.Close(); err != nil {
		t.Fatal(err)
	}

	<-ch
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

func startConsul(t *testing.T) *os.Process {
	cmd := exec.Command("consul", "agent", "-dev")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	go cmd.Wait()

	time.Sleep(100 * time.Millisecond)
	return cmd.Process
}

func stopConsul(t *testing.T, p *os.Process) {
	p.Kill()
}
