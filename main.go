package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MockEndpoint struct {
	Path            string            `json:"path"`
	Method          string            `json:"method"`
	ResponseBody    interface{}       `json:"response_body"`
	ResponseHeaders map[string]string `json:"response_headers"`
	StatusCode      int               `json:"status_code"`
	Delay           int               `json:"delay_ms"`
}

type MockServer struct {
	endpoints map[string]map[string]*MockEndpoint
	mu        sync.RWMutex
}

func NewMockServer() *MockServer {
	return &MockServer{
		endpoints: make(map[string]map[string]*MockEndpoint),
	}
}

func (s *MockServer) AddEndpoint(endpoint *MockEndpoint) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.endpoints[endpoint.Path] == nil {
		s.endpoints[endpoint.Path] = make(map[string]*MockEndpoint)
	}
	s.endpoints[endpoint.Path][endpoint.Method] = endpoint
}

func (s *MockServer) GetEndpoint(path, method string) (*MockEndpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if methods, exists := s.endpoints[path]; exists {
		if endpoint, methodExists := methods[method]; methodExists {
			return endpoint, true
		}
	}
	return nil, false
}

func (s *MockServer) ListEndpoints() []MockEndpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var endpoints []MockEndpoint
	for _, methods := range s.endpoints {
		for _, endpoint := range methods {
			endpoints = append(endpoints, *endpoint)
		}
	}
	return endpoints
}

func (s *MockServer) DeleteEndpoint(path, method string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if methods, exists := s.endpoints[path]; exists {
		if _, methodExists := methods[method]; methodExists {
			delete(methods, method)
			if len(methods) == 0 {
				delete(s.endpoints, path)
			}
			return true
		}
	}
	return false
}

func main() {
	var (
		host = flag.String("host", "0.0.0.0", "Host to bind to")
		port = flag.Int("port", 8080, "Port to listen on")
	)
	flag.Parse()

	server := NewMockServer()

	http.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		endpoints := server.ListEndpoints()
		adminPage(endpoints).Render(r.Context(), w)
	})

	http.HandleFunc("/admin/mocks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if strings.Contains(r.Header.Get("Accept"), "text/html") || r.Header.Get("HX-Request") == "true" {
				endpoints := server.ListEndpoints()
				endpointsList(endpoints).Render(r.Context(), w)
				return
			}

			endpoints := server.ListEndpoints()
			log.Info().Interface("endpoints", endpoints).Msg("Endpoints")
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(endpoints)
			if err != nil {
				log.Error().Err(err).Msg("Error encoding endpoints")
				return
			}

		case http.MethodPost:
			if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" || r.Header.Get("HX-Request") == "true" {
				err := r.ParseForm()
				if err != nil {
					http.Error(w, "Failed to parse form", http.StatusBadRequest)
					return
				}

				endpoint := MockEndpoint{
					Path:   r.FormValue("path"),
					Method: r.FormValue("method"),
				}

				if endpoint.Path == "" {
					http.Error(w, "Path is required", http.StatusBadRequest)
					return
				}

				if endpoint.Method == "" {
					endpoint.Method = http.MethodGet
				}

				if statusStr := r.FormValue("status_code"); statusStr != "" {
					if status, err := strconv.Atoi(statusStr); err == nil {
						endpoint.StatusCode = status
					}
				}
				if endpoint.StatusCode == 0 {
					endpoint.StatusCode = http.StatusOK
				}

				if delayStr := r.FormValue("delay_ms"); delayStr != "" {
					if delay, err := strconv.Atoi(delayStr); err == nil {
						endpoint.Delay = delay
					}
				}

				if headersStr := r.FormValue("response_headers"); headersStr != "" {
					var headers map[string]string
					if err := json.Unmarshal([]byte(headersStr), &headers); err == nil {
						endpoint.ResponseHeaders = headers
					}
				}

				if bodyStr := r.FormValue("response_body"); bodyStr != "" {
					var jsonBody interface{}
					if err := json.Unmarshal([]byte(bodyStr), &jsonBody); err == nil {
						endpoint.ResponseBody = jsonBody
					} else {
						endpoint.ResponseBody = bodyStr
					}
				}

				if endpoint.Delay > 2000 {
					endpoint.Delay = 2000
				}

				server.AddEndpoint(&endpoint)
				log.Info().Msgf("Mock endpoint added via web UI: [%s] %s", endpoint.Method, endpoint.Path)

				endpoints := server.ListEndpoints()
				endpointsList(endpoints).Render(r.Context(), w)
				return
			}

			var endpoint MockEndpoint
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read request body", http.StatusBadRequest)
				return
			}

			if err := json.Unmarshal(body, &endpoint); err != nil {
				http.Error(w, "Invalid JSON format", http.StatusBadRequest)
				return
			}

			if endpoint.Method == "" {
				endpoint.Method = http.MethodGet
			}
			if endpoint.StatusCode == 0 {
				endpoint.StatusCode = http.StatusOK
			}

			msg := fmt.Sprintf("Mock endpoint created: %s %s", endpoint.Method, endpoint.Path)

			if endpoint.Delay > 2000 {
				endpoint.Delay = 2000
				msg = msg + fmt.Sprintf("Your delay is set to: %d, as the maximum value", endpoint.Delay)
			}
			server.AddEndpoint(&endpoint)

			log.Info().Msgf("Mock endpoint added: [%s]", endpoint.Path)
			w.WriteHeader(http.StatusCreated)
			err = json.NewEncoder(w).Encode(map[string]string{
				"message": msg,
			})
			if err != nil {
				log.Error().Err(err).Msg("Error encoding response")
				return
			}

		case http.MethodDelete:
			path := r.URL.Query().Get("path")
			method := r.URL.Query().Get("method")
			if path == "" || method == "" {
				http.Error(w, "Path and method parameters are required", http.StatusBadRequest)
				return
			}

			if server.DeleteEndpoint(path, method) {
				log.Info().Msgf("Mock endpoint deleted: %s", path)

				if r.Header.Get("HX-Request") == "true" {
					endpoints := server.ListEndpoints()
					endpointsList(endpoints).Render(r.Context(), w)
					return
				}

				w.WriteHeader(http.StatusOK)
				err := json.NewEncoder(w).Encode(map[string]string{
					"message": fmt.Sprintf("Mock endpoint deleted: %s %s", method, path),
				})
				if err != nil {
					log.Error().Err(err).Msg("Failed to write response")
					return
				}
			} else {
				http.Error(w, "Endpoint not found", http.StatusNotFound)
			}

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/mocks" || r.URL.Path == "/admin" {
			return
		}

		endpoint, exists := server.GetEndpoint(r.URL.Path, r.Method)
		if !exists {
			http.Error(w, "Mock endpoint not found", http.StatusNotFound)
			return
		}

		log.Info().Str("endpoint", endpoint.Path).Str("method", endpoint.Method).Str("ip", r.RemoteAddr).Msg("Requested endpoint")

		if endpoint.Delay > 0 {
			time.Sleep(time.Duration(endpoint.Delay) * time.Millisecond)
		}

		for key, value := range endpoint.ResponseHeaders {
			w.Header().Set(key, value)
		}

		w.WriteHeader(endpoint.StatusCode)

		if endpoint.ResponseBody != nil {
			if str, ok := endpoint.ResponseBody.(string); ok {
				_, err := w.Write([]byte(str))
				if err != nil {
					log.Error().Err(err).Msg("Failed to write response")
					return
				}
			} else {
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(endpoint.ResponseBody)
				if err != nil {
					log.Error().Err(err).Msg("Failed to write response")
					return
				}
			}
		}
	})

	exampleEndpoints := []MockEndpoint{
		{
			Path:   "/api/v1/users",
			Method: http.MethodGet,
			ResponseBody: []map[string]interface{}{
				{"id": 1, "name": "John Doe", "email": "john@example.com"},
				{"id": 2, "name": "Jane Smith", "email": "jane@example.com"},
			},
			ResponseHeaders: map[string]string{
				"X-Total-Count": "2",
			},
			StatusCode: http.StatusOK,
			Delay:      500,
		},
		{
			Path:   "/api/v1/users",
			Method: http.MethodPost,
			ResponseBody: map[string]interface{}{
				"id":      3,
				"message": "User created successfully",
			},
			StatusCode: http.StatusCreated,
		},
		{
			Path:   "/api/v1/health",
			Method: http.MethodGet,
			ResponseBody: map[string]string{
				"status":  "healthy",
				"version": "1.0.0",
			},
			StatusCode: http.StatusOK,
		},
	}

	for _, endpoint := range exampleEndpoints {
		server.AddEndpoint(&endpoint)
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("Mock API Server starting on %s\n", addr)
	fmt.Println("\nWeb interface:")
	fmt.Printf("  http://%s/admin - Admin web interface\n", addr)
	fmt.Println("\nAdmin API endpoints:")
	fmt.Println("  GET    /admin/mocks - List all mock endpoints")
	fmt.Println("  POST   /admin/mocks - Create a new mock endpoint")
	fmt.Println("  DELETE /admin/mocks?path=/path&method=GET - Delete a mock endpoint")
	fmt.Println("\nExample endpoints created:")
	for _, endpoint := range exampleEndpoints {
		fmt.Printf("  %s %s (delay: %dms)\n", endpoint.Method, endpoint.Path, endpoint.Delay)
	}
	log.Fatal().Err(http.ListenAndServe(addr, nil))
}
