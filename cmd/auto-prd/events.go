package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	keepAliveInterval    = 20 * time.Second
	subscriberBufferSize = 128
)

type serverEvent struct {
	Type         string `json:"type"`
	RepoID       string `json:"repo_id,omitempty"`
	RepoPath     string `json:"repo_path,omitempty"`
	TicketNumber string `json:"ticket_number,omitempty"`
	Title        string `json:"title,omitempty"`
	Status       string `json:"status,omitempty"`
	JobID        string `json:"job_id,omitempty"`
	Action       string `json:"action,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Error        string `json:"error,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	subID, ch := s.addSubscriber()
	defer s.removeSubscriber(subID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case evt := <-ch:
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		}
	}
}

func (s *server) addSubscriber() (string, chan serverEvent) {
	id := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	ch := make(chan serverEvent, subscriberBufferSize)
	s.subsMu.Lock()
	s.subscribers[id] = ch
	s.subsMu.Unlock()
	return id, ch
}

func (s *server) removeSubscriber(id string) {
	s.subsMu.Lock()
	ch, ok := s.subscribers[id]
	if ok {
		delete(s.subscribers, id)
	}
	s.subsMu.Unlock()
	if ok {
		close(ch)
	}
}

func (s *server) broadcast(evt serverEvent) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}
