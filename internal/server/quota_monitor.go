package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Neokil/AutoPR/internal/providers"
)

const (
	quotaMonitorInterval    = 20 * time.Minute
	quotaMonitorInitialWait = 20 * time.Second
)

func (s *server) quotaMonitorLoop() {
	ticker := time.NewTicker(quotaMonitorInterval)

	defer ticker.Stop()
	time.Sleep(quotaMonitorInitialWait)
	for range ticker.C {
		s.checkQuotaStatus()
	}
}

func (s *server) isQuotaReached() bool {
	s.quotaMu.RLock()
	defer s.quotaMu.RUnlock()
	return s.quotaReached
}

func (s *server) setQuotaReached(reached bool) {
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	s.quotaReached = reached
}

func (s *server) checkQuotaStatus() {

	if !s.isQuotaReached() {
		return
	}
	slog.Info("quota monitor: probing provider to check if quota has reset")

	repos := s.meta.ListRepos()
	if len(repos) == 0 {
		slog.Warn("quota monitor: no repos available for probe, skipping")
		return
	}
	rt, err := s.runtimeForRepo(repos[0].Path)
	if err != nil {
		slog.Error("quota monitor: failed to get runtime for probe", "err", err)
		return
	}

	probeErr := rt.svc.ProbeProvider(context.Background())
	if errors.Is(probeErr, providers.ErrTokensExhausted) {
		slog.Info("quota monitor: quota still reached, will check again later")
		return
	}

	s.setQuotaReached(false)
	slog.Info("quota monitor: quota has reset, resuming operations")
}
