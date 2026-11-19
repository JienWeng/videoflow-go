package api

import (
	"context"
	"sync"
	"time"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/providers"
)

// keyCheckBase is where key checks are sent. Tests replace it so they never touch the network.
var keyCheckBase = func(provider, endpoint string) string { return endpoint }

var keyCheckRefresh sync.Mutex

// checkVideoKey asks a video service whether it accepts a key. It is free: see
// providers.CheckKey. Nothing is saved here.
func checkVideoKey(ctx context.Context, provider, key, endpoint string) providers.KeyCheck {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return providers.NewHostClient(provider, key, keyCheckBase(provider, endpoint)).CheckKey(ctx)
}

// refreshKeyChecks checks saved video keys that are due (see KeyCheckDue). It waits a
// few seconds so the screen that asked can show the answer; slower checks finish behind.
func (s *Server) refreshKeyChecks() {
	if !keyCheckRefresh.TryLock() {
		return
	}
	routes, now := s.routes(), time.Now()
	var wg sync.WaitGroup
	for _, host := range costs.Hosts {
		if !providers.VideoHost(host.Name) || !routes.KeyCheckDue(host.Name, now) {
			continue
		}
		key, endpoint, _, err := routes.Credentials(host.Name)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(provider, key, endpoint string) {
			defer wg.Done()
			_ = routes.SaveKeyCheck(provider, key, checkVideoKey(context.Background(), provider, key, endpoint))
		}(host.Name, key, endpoint)
	}
	done := make(chan struct{})
	go func() {
		defer keyCheckRefresh.Unlock()
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

// keyCheckView is the last answer about a video service's key, for screens; nil when none.
func (s *Server) keyCheckView(provider string) *providers.KeyCheck {
	if !providers.VideoHost(provider) {
		return nil
	}
	if check, ok := s.routes().KeyCheck(provider); ok {
		return &check
	}
	return nil
}
