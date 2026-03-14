package controller

import (
	"context"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func StartExposer(ctx context.Context, address string) {
	http.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr: address,
	}

	go func() {
		<-ctx.Done()
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("failed to start prometheus exporter: %v", err)
	}
}
