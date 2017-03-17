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

	c1, err := New(&Config{})
	if err != nil {
		t.Fatal(err)
	}

	//time.Sleep(time.Second)
	testNext(t, "fail", c1, 1, 0)

	// start service
	go func() {
		m := http.NewServeMux()
		m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		s := http.Server{Handler: m}
		s.Serve(lis)
	}()

	if err = c1.Close(); err != nil {
		t.Fatal(err)
	}

	ch := make(chan struct{})
	go func() {
		defer close(ch)

		c2, err := New(&Config{})
		if err != nil {
			t.Fatal(err)
		}

		testNext(t, "pass", c2, 0, 1)

		go func() {
			if err := c2.Close(); err != nil {
				t.Fatal(err)
			}
		}()

		_, err = c2.Next()
		if err != ErrClosed {
			t.Errorf("_, err = c.Next(); err = %v, want %v", err, ErrClosed)
		}
	}()

	<-ch
}

func testNext(t *testing.T, name string, c *Consul, ccl, pcl int) {
	hcs, err := c.Next()
	if err != nil {
		t.Fatal(err)
	}

	for status, want := range map[string]int{"critical": ccl, "passing": pcl} {
		l := 0
		for _, hc := range hcs {
			if hc.Status == status {
				l++
			}
		}

		if l != want {
			t.Errorf("%s: [%s] checks = %d, want %d", name, status, l, want)
		}
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
