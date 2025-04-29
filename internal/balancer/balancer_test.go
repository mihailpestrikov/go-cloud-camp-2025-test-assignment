package balancer

import (
	"context"
	"go-cloud-camp-2025-test-assignment/config"
	"net/url"
	"testing"
	"time"
)

func TestNewBackend(t *testing.T) {
	type args struct {
		backendURL string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "Valid URL",
			args:    args{backendURL: "http://example.com"},
			want:    "http://example.com",
			wantErr: false,
		},
		{
			name:    "Invalid URL",
			args:    args{backendURL: "://invalid-url"},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewBackend(tt.args.backendURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got.URL.String() != tt.want {
				t.Errorf("NewBackend() got URL = %v, want %v", got.URL.String(), tt.want)
			}
		})
	}
}

func TestBackend_ActiveConns(t *testing.T) {
	backend, err := NewBackend("http://example.com")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	if got := backend.GetActiveConns(); got != 0 {
		t.Errorf("Initial GetActiveConns() = %v, want 0", got)
	}

	backend.IncrementActiveConns()
	if got := backend.GetActiveConns(); got != 1 {
		t.Errorf("After increment GetActiveConns() = %v, want 1", got)
	}

	backend.DecrementActiveConns()
	if got := backend.GetActiveConns(); got != 0 {
		t.Errorf("After decrement GetActiveConns() = %v, want 0", got)
	}
}

func TestBackend_IsAvailable(t *testing.T) {
	backend, err := NewBackend("http://example.com")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	if !backend.IsAvailable() {
		t.Errorf("New backend should be available")
	}

	backend.MarkDown()
	if backend.IsAvailable() {
		t.Errorf("Backend should be unavailable after MarkDown()")
	}

	backend.MarkUp()
	if !backend.IsAvailable() {
		t.Errorf("Backend should be available after MarkUp()")
	}
}

func TestBackend_RecordRequest(t *testing.T) {
	backend, err := NewBackend("http://example.com")
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	backend.RecordRequest(true)
	if backend.TotalRequests.Load() != 1 {
		t.Errorf("TotalRequests should be 1, got %d", backend.TotalRequests.Load())
	}
	if backend.FailedReqs.Load() != 0 {
		t.Errorf("FailedReqs should be 0, got %d", backend.FailedReqs.Load())
	}

	backend.RecordRequest(false)
	if backend.TotalRequests.Load() != 2 {
		t.Errorf("TotalRequests should be 2, got %d", backend.TotalRequests.Load())
	}
	if backend.FailedReqs.Load() != 1 {
		t.Errorf("FailedReqs should be 1, got %d", backend.FailedReqs.Load())
	}
}

func TestBackend_GetStatus(t *testing.T) {
	u, _ := url.Parse("http://example.com")
	backend := &Backend{
		URL: u,
	}
	backend.IsAlive.Store(true)
	backend.ActiveConns.Store(5)
	backend.FailureCount.Store(2)
	backend.TotalRequests.Store(100)
	backend.FailedReqs.Store(10)

	now := time.Now()
	backend.LastChecked.Store(now)

	status := backend.GetStatus()

	if status.URL != u {
		t.Errorf("GetStatus() URL = %v, want %v", status.URL, u)
	}
	if status.IsAlive != true {
		t.Errorf("GetStatus() IsAlive = %v, want %v", status.IsAlive, true)
	}
	if status.ActiveConns != 5 {
		t.Errorf("GetStatus() ActiveConns = %v, want %v", status.ActiveConns, 5)
	}
	if status.FailureCount != 2 {
		t.Errorf("GetStatus() FailureCount = %v, want %v", status.FailureCount, 2)
	}
	if status.TotalRequests != 100 {
		t.Errorf("GetStatus() TotalRequests = %v, want %v", status.TotalRequests, 100)
	}
	if status.FailedReqs != 10 {
		t.Errorf("GetStatus() FailedReqs = %v, want %v", status.FailedReqs, 10)
	}
	if !status.LastChecked.Equal(now) {
		t.Errorf("GetStatus() LastChecked = %v, want %v", status.LastChecked, now)
	}
}

func TestBaseBalancer_RegisterAndRemoveBackend(t *testing.T) {
	balancer := NewBaseBalancer([]*Backend{})

	backend1, _ := NewBackend("http://example1.com")
	backend2, _ := NewBackend("http://example2.com")

	balancer.RegisterBackend(backend1)
	if len(balancer.GetAllBackends()) != 1 {
		t.Errorf("After registering one backend, GetAllBackends() len = %v, want 1", len(balancer.GetAllBackends()))
	}

	balancer.RegisterBackend(backend2)
	if len(balancer.GetAllBackends()) != 2 {
		t.Errorf("After registering two backends, GetAllBackends() len = %v, want 2", len(balancer.GetAllBackends()))
	}

	balancer.RegisterBackend(backend1)
	if len(balancer.GetAllBackends()) != 2 {
		t.Errorf("After registering duplicate backend, GetAllBackends() len = %v, want 2", len(balancer.GetAllBackends()))
	}

	balancer.RemoveBackend(backend1)
	if len(balancer.GetAllBackends()) != 1 {
		t.Errorf("After removing one backend, GetAllBackends() len = %v, want 1", len(balancer.GetAllBackends()))
	}

	nonExistentBackend, _ := NewBackend("http://nonexistent.com")
	balancer.RemoveBackend(nonExistentBackend)
	if len(balancer.GetAllBackends()) != 1 {
		t.Errorf("After removing non-existent backend, GetAllBackends() len = %v, want 1", len(balancer.GetAllBackends()))
	}
}

func TestBaseBalancer_GetHealthyBackends(t *testing.T) {
	backend1, _ := NewBackend("http://example1.com")
	backend2, _ := NewBackend("http://example2.com")
	backend3, _ := NewBackend("http://example3.com")

	backend2.MarkDown()

	balancer := NewBaseBalancer([]*Backend{backend1, backend2, backend3})

	healthy := balancer.GetHealthyBackends()
	if len(healthy) != 2 {
		t.Errorf("GetHealthyBackends() len = %v, want 2", len(healthy))
	}

	for _, b := range healthy {
		if b.URL.String() == "http://example2.com" {
			t.Errorf("GetHealthyBackends() contains down backend: %v", b.URL.String())
		}
	}
}

func TestBalancerFactory(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "Round Robin",
			algorithm: "round_robin",
			wantName:  "round_robin",
			wantErr:   false,
		},
		{
			name:      "Least Connections",
			algorithm: "least_connections",
			wantName:  "least_connections",
			wantErr:   false,
		},
		{
			name:      "Random",
			algorithm: "random",
			wantName:  "random",
			wantErr:   false,
		},
		{
			name:      "Default to Round Robin for Unknown",
			algorithm: "unknown",
			wantName:  "round_robin",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Backends: []config.BackendConfig{
					{URL: "http://example.com"},
				},
				Balancer: config.BalancerConfig{
					Algorithm: tt.algorithm,
				},
			}

			balancer, err := BalancerFactory(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("BalancerFactory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && balancer.Name() != tt.wantName {
				t.Errorf("BalancerFactory() balancer name = %v, want %v", balancer.Name(), tt.wantName)
			}
		})
	}

	t.Run("No valid backends", func(t *testing.T) {
		cfg := &config.Config{
			Backends: []config.BackendConfig{},
			Balancer: config.BalancerConfig{
				Algorithm: "round_robin",
			},
		}

		_, err := BalancerFactory(cfg)
		if err == nil {
			t.Errorf("BalancerFactory() with no backends should return error")
		}
	})
}

func TestLeastConnectionsBalancer_NextBackend(t *testing.T) {
	backend1, _ := NewBackend("http://example1.com")
	backend2, _ := NewBackend("http://example2.com")
	backend3, _ := NewBackend("http://example3.com")

	backend1.ActiveConns.Store(5)
	backend2.ActiveConns.Store(2)
	backend3.ActiveConns.Store(10)

	balancer := NewLeastConnectionsBalancer([]*Backend{backend1, backend2, backend3})

	backend, err := balancer.NextBackend()
	if err != nil {
		t.Fatalf("NextBackend() error = %v", err)
	}

	if backend.URL.String() != "http://example2.com" {
		t.Errorf("NextBackend() got URL = %v, want http://example2.com", backend.URL.String())
	}
}

func TestRandomBalancer_NextBackend(t *testing.T) {
	backend1, _ := NewBackend("http://example1.com")
	backend2, _ := NewBackend("http://example2.com")
	backend3, _ := NewBackend("http://example3.com")

	balancer := NewRandomBalancer([]*Backend{backend1, backend2, backend3})

	backend, err := balancer.NextBackend()
	if err != nil {
		t.Fatalf("NextBackend() error = %v", err)
	}

	if backend == nil {
		t.Errorf("NextBackend() returned nil backend")
	}
}

type MockHealthChecker struct {
	checkResults map[string]bool
}

func (m *MockHealthChecker) Check(ctx context.Context, backend *Backend) bool {
	return m.checkResults[backend.URL.String()]
}

func TestStartHealthChecks(t *testing.T) {

	backend1, _ := NewBackend("http://example1.com")
	backend2, _ := NewBackend("http://example2.com")

	balancer := NewRoundRobinBalancer([]*Backend{backend1, backend2})

	checker := &MockHealthChecker{
		checkResults: map[string]bool{
			"http://example1.com": true,
			"http://example2.com": false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cfg := &config.HealthCheckConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond,
		Path:     "/health",
	}

	StartHealthChecks(ctx, balancer, cfg, checker)

	time.Sleep(150 * time.Millisecond)

	if !backend1.IsAvailable() {
		t.Errorf("backend1 should be available")
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for backend2.IsAvailable() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if backend2.IsAvailable() {
		t.Errorf("backend2 should not be available")
	}
}
