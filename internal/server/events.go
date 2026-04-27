package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Neokil/AutoPR/internal/api"
)

const (
	keepAliveInterval    = 20 * time.Second
	subscriberBufferSize = 128
)

func (s *server) handleEvents(resp http.ResponseWriter, req *http.Request) {
	flusher, ok := resp.(http.Flusher)
	if !ok {
		writeError(resp, http.StatusInternalServerError, "streaming unsupported")

		return
	}
	subID, eventCh := s.addSubscriber()
	defer s.removeSubscriber(subID)

	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.WriteHeader(http.StatusOK)
	flusher.Flush()

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()

	for {
		select {
		case <-req.Context().Done():
			return
		case <-keepAlive.C:
			_, _ = fmt.Fprint(resp, ": keepalive\n\n")
			flusher.Flush()
		case evt := <-eventCh:
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(resp, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		}
	}
}

func (s *server) addSubscriber() (string, chan api.ServerEvent) {
	id := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	eventCh := make(chan api.ServerEvent, subscriberBufferSize)
	s.subsMu.Lock()
	s.subscribers[id] = eventCh
	s.subsMu.Unlock()

	return id, eventCh
}

func (s *server) removeSubscriber(id string) {
	s.subsMu.Lock()
	eventCh, ok := s.subscribers[id]
	if ok {
		delete(s.subscribers, id)
	}
	s.subsMu.Unlock()
	if ok {
		close(eventCh)
	}
}

func (s *server) broadcast(evt api.ServerEvent) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}
