package goapplib

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/felixge/httpsnoop"
)

// WebAppServer provides a generic HTTP server with optional CORS and logging.
type WebAppServer struct {
	Address       string
	GrpcAddress   string
	AllowLocalDev bool
}

// StartWithHandler starts the HTTP server with the given handler.
func (s *WebAppServer) StartWithHandler(ctx context.Context, handler http.Handler, srvErr chan error, stopChan chan bool) error {
	if s.AllowLocalDev {
		PrintStartupMessage(s.Address)
	} else {
		log.Println("Starting http web server on: ", s.Address)
	}
	handler = withLogger(handler)
	if s.AllowLocalDev {
		handler = CORS(handler)
	}
	server := &http.Server{
		Addr:        s.Address,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
		Handler:     handler,
	}

	go func() {
		<-stopChan
		if err := server.Shutdown(context.Background()); err != nil {
			log.Fatalln(err)
			panic(err)
		}
	}()
	srvErr <- server.ListenAndServe()
	return nil
}

func withLogger(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		m := httpsnoop.CaptureMetrics(handler, writer, request)
		if false && m.Code != 200 { // turn off frequent logs
			log.Printf("http[%d]-- %s -- %s, Query: %s\n", m.Code, m.Duration, request.URL.Path, request.URL.RawQuery)
		}
	})
}

// CORS adds CORS headers for local development.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, Origin, Cache-Control, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Methods", "PUT, DELETE")

		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorCyan   = "\033[36m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBold   = "\033[1m"
)

// makeClickableLink creates a clickable terminal link using OSC 8 escape sequences
func makeClickableLink(url string, color string) string {
	return fmt.Sprintf("\033]8;;%s\033\\%s%s%s\033]8;;\033\\", url, color, url, ColorReset)
}

// GetLocalIP returns the local network IP address (preferring ethernet/wifi over localhost)
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no local network IP found")
}

// PrintStartupMessage prints a colorized startup message with clickable links
func PrintStartupMessage(address string) {
	port := address
	if strings.HasPrefix(port, ":") {
		port = port[1:]
	} else {
		parts := strings.Split(address, ":")
		if len(parts) > 0 {
			port = parts[len(parts)-1]
		}
	}

	fmt.Println()
	fmt.Printf("%s%s╔════════════════════════════════════════════════════════════╗%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Printf("%s%s║  Server started! Open in your browser:                     ║%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Printf("%s%s╠════════════════════════════════════════════════════════════╣%s\n", ColorBold, ColorCyan, ColorReset)

	localhostURL := fmt.Sprintf("http://localhost:%s", port)
	clickableLocalhost := makeClickableLink(localhostURL, ColorGreen)
	spaces := max(0, 60-len(localhostURL)) - 2
	fmt.Printf("%s%s║  %s%s%s%s║%s\n",
		ColorBold, ColorCyan, clickableLocalhost, ColorCyan, ColorBold, strings.Repeat(" ", spaces), ColorReset)

	if localIP, err := GetLocalIP(); err == nil {
		networkURL := fmt.Sprintf("http://%s:%s", localIP, port)
		clickableNetwork := makeClickableLink(networkURL, ColorYellow)
		spaces := max(0, 60-len(networkURL)) - 2
		fmt.Printf("%s%s║  %s%s%s%s║%s\n",
			ColorBold, ColorCyan, clickableNetwork, ColorCyan, ColorBold, strings.Repeat(" ", spaces), ColorReset)
	}

	fmt.Printf("%s%s╚════════════════════════════════════════════════════════════╝%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Println()
}
