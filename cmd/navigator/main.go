package main

import (
	"flag"

	"github.com/avito-tech/navigator/cmd/navigator/app"
)

func main() {
	var config app.Config

	flag.StringVar(&config.ServingAddress, "address", "0.0.0.0", "Navigator GRPC serving address")
	flag.StringVar(&config.GRPCPort, "port", "8001", "Navigator GRPC serving port")
	flag.StringVar(&config.HTTPPort, "http-port", "8089", "Healthcheck and prometheus metrics serving port")
	flag.StringVar(&config.Loglevel, "log-level", "warn", "Logger level from trace to panic")
	flag.BoolVar(&config.LogJSONFormatter, "log-json", false, "Enable JSON formatter for logger")
	flag.BoolVar(&config.EnableProfiling, "profiling", false, "Enable pprof endpoints")
	flag.BoolVar(&config.EnableSidecarHealthCheck, "sidecar-health", true, "Enable custon health check endpoint configuration for sidecar")
	flag.BoolVar(&config.LocalityEnabled, "enable-locality", false, "Enable locality-aware routing")
	flag.StringVar(&config.SidecarHealthCheckPath, "sidecar-health-path", "health", "Sidecar health check path")
	flag.IntVar(&config.SidecarHealthCheckPort, "sidecar-health-port", 7313, "Sidecar health check port")

	flag.Parse()

	config.KubeConfigs = append(config.KubeConfigs, flag.Args()...)

	app.Run(config)
}
