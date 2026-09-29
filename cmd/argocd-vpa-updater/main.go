// Command argocd-vpa-updater runs the controller (watching opted-in
// VerticalPodAutoscaler objects), the write-back worker and the dashboard
// API side by side in one process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/azurebrasil/argocd-vpa-updater/internal/api"
	gitopsv1alpha1 "github.com/azurebrasil/argocd-vpa-updater/internal/apis/vpagitopsbinding/v1alpha1"
	"github.com/azurebrasil/argocd-vpa-updater/internal/argocdapp"
	"github.com/azurebrasil/argocd-vpa-updater/internal/auth"
	"github.com/azurebrasil/argocd-vpa-updater/internal/config"
	"github.com/azurebrasil/argocd-vpa-updater/internal/controller"
	"github.com/azurebrasil/argocd-vpa-updater/internal/credentials"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitexec"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitwriteback"
	"github.com/azurebrasil/argocd-vpa-updater/internal/patcher"
	"github.com/azurebrasil/argocd-vpa-updater/internal/resolver"
	"github.com/azurebrasil/argocd-vpa-updater/internal/statestore"
	"github.com/azurebrasil/argocd-vpa-updater/internal/version"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vpaapi"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vparecommendation"
	"github.com/azurebrasil/argocd-vpa-updater/internal/workloadresources"
	"github.com/azurebrasil/argocd-vpa-updater/internal/writebackworker"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	logger.Info("starting argocd-vpa-updater", "version", version.Version, "commit", version.GitCommit)

	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	scheme := clientgoscheme.Scheme
	if err := vpaapi.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register VerticalPodAutoscaler scheme: %w", err)
	}
	if err := gitopsv1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register VpaGitOpsBinding scheme: %w", err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		// Disabled: controller-runtime's default metrics bind address
		// (:8080) would otherwise collide with our own API server, which
		// owns that port (see cfg.ListenAddr).
		Metrics: metricsserver.Options{BindAddress: "0"},
		Client: client.Options{
			// The manager's default client caches objects via list+watch
			// informers, which would require cluster-wide list/watch on
			// Secrets -- far broader than the minimal, resourceName-scoped
			// RBAC this controller is granted (see deploy/manifests). Secret
			// reads/writes go straight to the API server instead, using only
			// the get/update/list verbs the Roles actually grant. The same
			// reasoning applies to the workload kinds the dashboard reads
			// current resource values from (internal/workloadresources):
			// only "get" is granted, so caching them would need a
			// cluster-wide watch RBAC doesn't have -- a single uncached read
			// per request is still far cheaper than the Git clone it
			// replaces.
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{&corev1.Secret{}, &appsv1.Deployment{}, &appsv1.StatefulSet{}, &batchv1.CronJob{}},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create controller manager: %w", err)
	}

	cache := vparecommendation.NewCache()
	if err := controller.AddToManager(mgr, cache); err != nil {
		return fmt.Errorf("register VPA controller: %w", err)
	}

	svc := &api.Service{
		Reader:         vparecommendation.NewCacheReader(cache),
		Resolver:       resolver.NewCRDResolver(argocdapp.NewRepoURLGetter(mgr.GetClient()), cfg.ArgoCDNamespace),
		WorkloadReader: workloadresources.NewClusterReader(mgr.GetClient()),
		State:          statestore.NewSecretStore(mgr.GetClient(), cfg.StateSecretNamespace, cfg.StateSecretName),
	}
	var serverOpts []api.Option
	if cfg.Auth.Enabled {
		authenticator, err := auth.New(auth.Config{
			Username:     cfg.Auth.Username,
			PasswordHash: cfg.Auth.PasswordHash,
			SigningKey:   []byte(cfg.Auth.SigningKey),
			SessionTTL:   cfg.Auth.SessionTTL,
		})
		if err != nil {
			return fmt.Errorf("configure authentication (set ADMIN_PASSWORD_HASH and SESSION_SIGNING_KEY, or AUTH_ENABLED=false for local development): %w", err)
		}
		serverOpts = append(serverOpts, api.WithAuth(authenticator, cfg.Auth.CookieSecure))
		logger.Info("authentication enabled", "username", cfg.Auth.Username)
	} else {
		logger.Warn("authentication disabled (AUTH_ENABLED=false): the dashboard and API are open to anyone who can reach them")
	}
	server := api.NewServer(svc, logger, serverOpts...)

	writeBackWorker := &writebackworker.Worker{
		State:        statestore.NewSecretStore(mgr.GetClient(), cfg.StateSecretNamespace, cfg.StateSecretName),
		WriteBack:    gitwriteback.NewCLIGitWriteBackService(gitexec.NewRunner()),
		Credentials:  credentials.NewArgoCDSecretProvider(mgr.GetClient(), cfg.ArgoCDNamespace),
		Patchers:     patcher.DefaultRegistry(),
		Logger:       logger,
		PollInterval: cfg.WriteBackPollInterval,
	}
	if err := mgr.Add(writeBackWorker); err != nil {
		return fmt.Errorf("register write-back worker: %w", err)
	}

	httpServer := &http.Server{Addr: cfg.ListenAddr, Handler: server}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)
	go func() {
		logger.Info("http server listening", "addr", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()
	go func() {
		logger.Info("controller manager starting")
		if err := mgr.Start(ctx); err != nil {
			errCh <- fmt.Errorf("controller manager: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		stop()
		_ = httpServer.Close()
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http server shutdown: %w", err)
	}
	return nil
}
