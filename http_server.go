package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

// SSEClient represents a connected SSE client
type SSEClient struct {
	ch chan string
}

// SSEHub manages SSE connections
type SSEHub struct {
	mu      sync.RWMutex
	clients map[*SSEClient]struct{}
}

func newSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[*SSEClient]struct{}),
	}
}

func (h *SSEHub) subscribe() *SSEClient {
	c := &SSEClient{ch: make(chan string, 64)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *SSEHub) unsubscribe(c *SSEClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.ch)
}

func (h *SSEHub) closeAll() {
	h.mu.Lock()
	for c := range h.clients {
		close(c.ch)
		delete(h.clients, c)
	}
	h.mu.Unlock()
}

func (h *SSEHub) broadcast(name string, data interface{}) {
	payload, err := json.Marshal(map[string]interface{}{
		"event": name,
		"data":  data,
	})
	if err != nil {
		return
	}
	msg := string(payload)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.ch <- msg:
		default:
			// drop if client is slow
		}
	}
}

// HTTPServer handles HTTP mode
type HTTPServer struct {
	app   *App
	hub   *SSEHub
	srv   *http.Server
	token string
	host  string
	port  int
}

// NewHTTPServer creates a new HTTP server instance
func NewHTTPServer(app *App, host string, port int, token string) *HTTPServer {
	return &HTTPServer{
		app:   app,
		hub:   newSSEHub(),
		token: token,
		host:  host,
		port:  port,
	}
}

// broadcast sends an event to all SSE clients
func (s *HTTPServer) broadcast(name string, data interface{}) {
	s.hub.broadcast(name, data)
}

// GenerateToken generates a random token
func GenerateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Start starts the HTTP server
func (s *HTTPServer) Start(staticFS fs.FS) error {
	mux := http.NewServeMux()

	// Auth middleware helper
	checkAuth := func(r *http.Request) bool {
		if s.token == "" {
			return true
		}
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.token {
			return true
		}
		return r.URL.Query().Get("token") == s.token
	}

	// POST /api/call — dispatch to App methods
	mux.HandleFunc("/api/call", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(r) {
			http.Error(w, "Unauthorized", 401)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}

		var req struct {
			Method string            `json:"method"`
			Args   []json.RawMessage `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		result, err := s.dispatch(req.Method, req.Args)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	})

	// GET /api/events — SSE stream
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(r) {
			http.Error(w, "Unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		client := s.hub.subscribe()
		defer s.hub.unsubscribe(client)

		// Send ping immediately to confirm connection
		fmt.Fprintf(w, "data: {\"event\":\"connected\"}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		for {
			select {
			case msg, ok := <-client.ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", msg)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-r.Context().Done():
				return
			}
		}
	})

	// Login check endpoint
	mux.HandleFunc("/api/auth", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if checkAuth(r) {
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		} else {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]bool{"ok": false})
		}
	})

	// File upload endpoint for browser mode
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(r) {
			http.Error(w, "Unauthorized", 401)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		r.ParseMultipartForm(32 << 20) // 32MB max
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file provided", 400)
			return
		}
		defer file.Close()

		// Save to temp directory
		tmpDir := os.TempDir()
		tmpFile, err := os.CreateTemp(tmpDir, "redc-upload-*-"+header.Filename)
		if err != nil {
			http.Error(w, "Failed to create temp file", 500)
			return
		}
		defer tmpFile.Close()
		io.Copy(tmpFile, file)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"path": tmpFile.Name()})
	})

	// Static files (SPA fallback)
	subFS, err := fs.Sub(staticFS, "frontend/dist")
	if err != nil {
		return fmt.Errorf("failed to access static files: %w", err)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		// Try to open the file
		f, err := subFS.Open(path)
		if err != nil {
			// SPA fallback: serve index.html for unknown paths
			path = "index.html"
			f, err = subFS.Open(path)
			if err != nil {
				http.Error(w, "Not found", 404)
				return
			}
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			// If it's a directory, try index.html inside it or fallback
			f.Close()
			path = "index.html"
			f, err = subFS.Open(path)
			if err != nil {
				http.Error(w, "Not found", 404)
				return
			}
			defer func() { /* already deferred above, but overwritten */ }()
			stat, _ = f.Stat()
		}

		// Determine content type from extension
		contentType := "application/octet-stream"
		if strings.HasSuffix(path, ".html") {
			contentType = "text/html; charset=utf-8"
		} else if strings.HasSuffix(path, ".js") {
			contentType = "application/javascript"
		} else if strings.HasSuffix(path, ".css") {
			contentType = "text/css"
		} else if strings.HasSuffix(path, ".json") {
			contentType = "application/json"
		} else if strings.HasSuffix(path, ".svg") {
			contentType = "image/svg+xml"
		} else if strings.HasSuffix(path, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(path, ".woff2") {
			contentType = "font/woff2"
		} else if strings.HasSuffix(path, ".woff") {
			contentType = "font/woff"
		}
		w.Header().Set("Content-Type", contentType)

		if rs, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, path, stat.ModTime(), rs)
		} else {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
			io.Copy(w, f)
		}
	})

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	s.srv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[HTTP Server] Error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the HTTP server
func (s *HTTPServer) Stop() error {
	if s.srv == nil {
		return nil
	}
	// Close all SSE clients first so Shutdown doesn't block on long-lived connections
	s.hub.closeAll()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := s.srv.Shutdown(ctx)
	if err != nil {
		// Force close if graceful shutdown times out
		s.srv.Close()
		err = nil
	}
	return err
}

// blockedInHTTPMode lists methods that require native OS dialogs
var blockedInHTTPMode = map[string]bool{
	"SelectFile":      true,
	"SelectDirectory": true,
	"SelectSaveFile":  true,
}

// dispatch calls an App method by name using reflection
func (s *HTTPServer) dispatch(method string, args []json.RawMessage) (interface{}, error) {
	if blockedInHTTPMode[method] {
		return nil, fmt.Errorf("此功能在浏览器模式下不可用，请使用桌面应用")
	}

	appVal := reflect.ValueOf(s.app)
	m := appVal.MethodByName(method)
	if !m.IsValid() {
		return nil, fmt.Errorf("method %s not found", method)
	}

	mt := m.Type()
	if mt.NumIn() != len(args) {
		return nil, fmt.Errorf("method %s expects %d args, got %d", method, mt.NumIn(), len(args))
	}

	in := make([]reflect.Value, len(args))
	for i, arg := range args {
		paramType := mt.In(i)
		paramPtr := reflect.New(paramType)
		if err := json.Unmarshal(arg, paramPtr.Interface()); err != nil {
			return nil, fmt.Errorf("arg %d: %w", i, err)
		}
		in[i] = paramPtr.Elem()
	}

	out := m.Call(in)

	if len(out) == 0 {
		return nil, nil
	}

	// Check if last return is error
	last := out[len(out)-1]
	if last.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		if !last.IsNil() {
			return nil, last.Interface().(error)
		}
		out = out[:len(out)-1]
	}

	if len(out) == 0 {
		return nil, nil
	}
	if len(out) == 1 {
		return out[0].Interface(), nil
	}

	// Multiple return values → return as array
	results := make([]interface{}, len(out))
	for i, v := range out {
		results[i] = v.Interface()
	}
	return results, nil
}
