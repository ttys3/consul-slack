package consul

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
)

func TestConsul_All(t *testing.T) {
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
			{
				Notes:    "TCP Check",
				TCP:      ":" + strconv.Itoa(port),
				Interval: "1s",
				Timeout:  "1s",
			},
			{
				Notes:  "Always true",
				Script: "/usr/bin/true",
			},
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	c1, err := New(WithLogger(log.New(os.Stderr, "[consul_1]", 0)))
	if err != nil {
		t.Fatal(err)
	}

	testNext(t, c1, Critical)
	if err = c1.Close(); err != nil {
		t.Fatal(err)
	}
	testClosed(t, c1)

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
		defer close(ch)

		c2, err := New(WithLogger(log.New(os.Stderr, "[consul_2]", 0)))
		if err != nil {
			t.Fatal(err)
		}

		testNext(t, c2, Passing)
		go func() {
			if err := c2.Close(); err != nil {
				t.Fatal(err)
			}
			testClosed(t, c2)
		}()

		ev := <-c2.C
		if ev != nil {
			t.Errorf("ev = %v, want nil", ev)
		}
	}()

	<-ch
}

func testNext(t *testing.T, c *Consul, status string) {
	hc := <-c.C
	if hc.Status != status {
		t.Errorf("Status = %q, want %q", hc.Status, status)
	}
}

func testClosed(t *testing.T, c *Consul) {
	select {
	case _, ok := <-c.C:
		if ok {
			t.Error("c.C is not empty")
		}
	default:
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
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
}
