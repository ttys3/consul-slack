package consul

import (
	"io/ioutil"
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

	c, err := New(WithLogger(log.New(ioutil.Discard, "", 0)))
	if err != nil {
		t.Fatal(err)
	}

	// server is not started at this point
	// so we expect it be a critical state
	testNext(t, c, Critical)

	// start service
	go func() {
		m := http.NewServeMux()
		m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		s := http.Server{Handler: m}
		s.Serve(lis)
	}()

	testNext(t, c, Passing)
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}

	_, ok := <-c.C
	if ok {
		t.Error("c.C is not closed")
	}
}

func testNext(t *testing.T, c *Consul, status string) {
	hc, ok := <-c.C
	if !ok {
		t.Fatal("closed already")
	}

	if hc.Status != status {
		t.Errorf("Status = %q, want %q", hc.Status, status)
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
