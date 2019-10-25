package app

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/avito-tech/navigator/pkg/apis/generated/clientset/versioned"
	"github.com/avito-tech/navigator/pkg/grpc"
	"github.com/avito-tech/navigator/pkg/handlers"
	"github.com/avito-tech/navigator/pkg/k8s"
	"github.com/avito-tech/navigator/pkg/xds"
	"github.com/avito-tech/navigator/pkg/xds/resources"
)

type Config struct {
	KubeConfigs              []string
	ServingAddress           string
	GRPCPort                 string
	HTTPPort                 string
	Loglevel                 string
	LogJSONFormatter         bool
	EnableProfiling          bool
	EnableSidecarHealthCheck bool
	SidecarHealthCheckPath   string
	SidecarHealthCheckPort   int
}

func Run(config Config) {
	logger, err := getLogger(config.Loglevel, config.LogJSONFormatter)
	if err != nil {
		logrus.WithError(err).Fatal("cannot create logger")
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	setSigintHandler(logger, cancelFunc)

	k8sCache := k8s.NewCache(logger)
	nexusCache := k8s.NewNexusCache()

	opts := getResourcesOpts(config)
	xdsCache := xds.NewCache(k8sCache, nexusCache, logger, opts)

	if len(config.KubeConfigs) == 0 {
		logger.Fatalf("At least 1 kubeconfig must be specified")
	}

	canary := k8s.NewCanary()
	for _, kubeConfig := range config.KubeConfigs {
		cs, err := getK8sClient(kubeConfig)
		if err != nil {
			logger.WithError(err).Fatalf("Failed to create k8s client with config %q", kubeConfig)
		}
		extCs, err := getExtK8sClient(kubeConfig)
		if err != nil {
			logger.WithError(err).Fatalf("Failed to create k8s cdr client with config %q", kubeConfig)
		}

		k8sWatcher := k8s.NewWatcher(
			logger,
			cs,
			extCs,
			k8sCache,
			canary,
			xdsCache,
			nexusCache,
			kubeConfig,
		)
		k8sWatcher.Start(ctx)
	}

	fullAddr := net.JoinHostPort(config.ServingAddress, config.GRPCPort)
	l, err := net.Listen("tcp", fullAddr)
	if err != nil {
		logger.WithError(err).Fatalf("Failed to listen %q", fullAddr)
	}

	grpcServer := grpc.NewAPI(logger, xdsCache)
	go func() {
		err = grpcServer.Serve(l)
		if err != nil {
			logger.Errorf("Failed to serve GRPC: %v", err)
		}
	}()

	mux := http.NewServeMux()
	handlers.RegisterMetrics(mux)
	handlers.RegisterHealthCheck(mux)
	xdsCache.RegisterDebugXDS(mux)
	if config.EnableProfiling {
		handlers.RegisterProfile(mux)
	}

	httpServer := &http.Server{
		Addr:    net.JoinHostPort(config.ServingAddress, config.HTTPPort),
		Handler: mux,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			logger.Errorf("Failed to serve metrics: %v", err)
		}
	}()

	<-ctx.Done()
	grpcServer.Stop()

	httpCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(httpCtx)
}

func getLogger(level string, json bool) (*logrus.Logger, error) {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return nil, err
	}
	logger := logrus.StandardLogger()
	logger.SetLevel(lvl)
	if json {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}
	return logger, nil
}

func getK8sClient(kubeConfig string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	client, err := kubernetes.NewForConfig(config)
	return client, err
}

func getExtK8sClient(kubeConfig string) (*versioned.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	client, err := versioned.NewForConfig(config)
	return client, err
}

func setSigintHandler(logger logrus.FieldLogger, cancelFunc context.CancelFunc) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)

	go func() {
		<-sigs
		logger.Info("SIGINT signal caught, stopping...")
		cancelFunc()
	}()
}

func getResourcesOpts(config Config) []resources.FuncOpt {
	opts := []resources.FuncOpt{}
	if dynamicClusterName, ok := os.LookupEnv("NAVIGATOR_DYNAMIC_CLUSTER"); ok {
		opts = append(opts, resources.WithClusterName(dynamicClusterName))
	}
	if config.EnableSidecarHealthCheck {
		opts = append(opts, resources.WithHealthCheck(config.SidecarHealthCheckPath, config.SidecarHealthCheckPort))
	}
	return opts
}
