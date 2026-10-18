package main

import (
	"errors"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/icap"
	"github.com/yatuk/tamga/internal/proxy"
)

// startICAP starts the ICAP server when TAMGA_ICAP_ADDR is set, and returns
// nil when it is not. scanAPI is the scan API's handler: the ICAP route
// scans with its configuration. See docs/icap.md.
func startICAP(cfg *config.Config, scanAPI http.Handler) *icap.Server {
	if cfg.ICAPAddr == "" {
		return nil
	}
	h, err := proxy.NewICAPHandler(scanAPI)
	if err != nil {
		log.Fatal().Err(err).Msg("icap: handler")
	}
	allowed := cfg.ICAPAllowedClients
	srv := &icap.Server{
		Handler:        h,
		MaxConnections: cfg.ICAPMaxConnections,
		// ICAP has no authentication of its own: only the web proxies
		// listed may connect, and with none listed only this host.
		AllowClient: func(ip net.IP) bool {
			if len(allowed) == 0 {
				return ip.IsLoopback()
			}
			for _, n := range allowed {
				if n.Contains(ip) {
					return true
				}
			}
			return false
		},
		OnError: func(remote string, err error) {
			log.Warn().Str("component", "icap").Str("icap_client", remote).Err(err).Msg("icap connection ended with an error")
		},
	}
	ln, err := net.Listen("tcp", cfg.ICAPAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", cfg.ICAPAddr).Msg("icap: listen")
	}
	go func() {
		log.Info().Str("addr", cfg.ICAPAddr).Int("allowed_client_ranges", len(allowed)).Msg("icap server starting")
		if err := srv.Serve(ln); err != nil && !errors.Is(err, icap.ErrServerClosed) {
			log.Fatal().Err(err).Msg("icap server failed")
		}
	}()
	return srv
}
