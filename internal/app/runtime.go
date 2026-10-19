package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	auditadapter "github.com/76Parker/metrico/internal/adapters/audit"
	"github.com/76Parker/metrico/internal/adapters/filestorage"
	"github.com/76Parker/metrico/internal/adapters/memstorage"
	"github.com/76Parker/metrico/internal/adapters/postgres"
	"github.com/76Parker/metrico/internal/api"
	"github.com/76Parker/metrico/internal/api/handlers"
	"github.com/76Parker/metrico/internal/applications/audit"
	"github.com/76Parker/metrico/internal/applications/health"
	"github.com/76Parker/metrico/internal/applications/metrics"
	"github.com/76Parker/metrico/internal/config"
	"github.com/76Parker/metrico/pkg/logger"
)

// running описывает сущности которые могут быть запущены и работать в фоновом режиме
type runner interface {
	Run(ctx context.Context) error
}

type application struct {
	server            *http.Server
	diagnostics       *http.Server
	backgroundWorkers []runner

	cleanupStack cleanupStack

	shutdownTimeout time.Duration
	log             logger.Logger
}

type backend struct {
	repository   metrics.Repository
	healthPinger healthPinger
	snapshotter  *metrics.SnapshotCoordinator
}

type healthPinger interface {
	Ping(ctx context.Context) error
}

type alwaysReadyPinger struct{}

func (alwaysReadyPinger) Ping(context.Context) error {
	return nil
}

func newApplication(ctx context.Context, cfg config.Config, log logger.Logger) (_ *application, err error) {
	app := &application{
		log:             log,
		cleanupStack:    cleanupStack{},
		shutdownTimeout: 10 * time.Second,
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, app.cleanupStack.close())
		}
	}()
	app.diagnostics, err = api.NewDiagnosticsServer(cfg.PPROFAddress)
	if err != nil {
		return nil, err
	}

	auditPublisher, err := newAuditPublisher(cfg.Audit, log, &app.cleanupStack)
	if err != nil {
		return nil, err
	}

	storage, err := newBackend(ctx, cfg, &app.cleanupStack)
	if err != nil {
		return nil, err
	}

	metricsOptions := make([]metrics.ApplicationOption, 0, 1)
	if storage.snapshotter != nil {
		metricsOptions = append(metricsOptions, metrics.WithSnapshotter(storage.snapshotter))
		if cfg.SnapshotServiceConfig.StoreInterval > 0 {
			app.backgroundWorkers = append(app.backgroundWorkers, storage.snapshotter)
		}
	}

	metricsApplication := metrics.NewApplication(
		metrics.NewService(storage.repository),
		metricsOptions...,
	)
	healthApplication := health.NewApplication(storage.healthPinger)
	handlerOptions := make([]handlers.MetricsHandlerOption, 0, 1)
	if auditPublisher != nil {
		handlerOptions = append(handlerOptions, handlers.WithAuditPublisher(auditPublisher))
	}

	app.server = api.NewRouter(
		handlers.NewMetricsHandler(metricsApplication, handlerOptions...),
		handlers.NewHealthHandler(log, healthApplication),
		cfg.HTTPConfig,
		log,
	)

	return app, nil
}

func newAuditPublisher(cfg config.Audit, log logger.Logger, cleanups *cleanupStack) (*audit.Publisher, error) {
	if cfg.File == "" && cfg.URL == "" {
		return nil, nil
	}
	observers := make([]audit.Observer, 0, 2)
	if cfg.File != "" {
		file, err := auditadapter.NewFile(cfg.File)
		if err != nil {
			return nil, err
		}
		cleanups.add(file.Close)
		observers = append(observers, file)
	}
	if cfg.URL != "" {
		client := &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
			Timeout:   time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		remote, err := auditadapter.NewHTTP(cfg.URL, client)
		if err != nil {
			return nil, err
		}
		cleanups.add(remote.Close)
		observers = append(observers, remote)
	}
	return audit.NewPublisher(log, observers...)
}

func (a *application) run(ctx context.Context) error {
	group, groupCtx := errgroup.WithContext(ctx)

	servers := []*http.Server{a.server}
	if a.diagnostics != nil {
		servers = append(servers, a.diagnostics)
	}
	for _, server := range servers {
		group.Go(func() error {
			return a.startHTTPServer(server)
		})
	}

	// Запускаем фоновые сервисы в отдельных горутинах
	for _, worker := range a.backgroundWorkers {
		worker := worker
		group.Go(func() error {
			err := worker.Run(groupCtx)
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		})
	}

	// Ожидаем сигнала завершения и выключаем HTTP сервер
	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
		defer cancel()
		var shutdowns errgroup.Group
		for _, server := range servers {
			shutdowns.Go(func() error {
				a.log.Info("Shutting down HTTP server", "addr", server.Addr)
				if err := server.Shutdown(shutdownCtx); err != nil {
					return errors.Join(fmt.Errorf("shutdown http %s: %w", server.Addr, err), server.Close())
				}
				return nil
			})
		}
		return shutdowns.Wait()
	})
	return group.Wait()
}

func (a *application) startHTTPServer(server *http.Server) error {
	a.log.Info("Starting HTTP server", "addr", server.Addr)
	err := server.ListenAndServe()
	if err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve http %s: %w", server.Addr, err)
	}
	return nil
}

func (a *application) close() error {
	return a.cleanupStack.close()
}

func newBackend(ctx context.Context, cfg config.Config, cleanups *cleanupStack) (backend, error) {
	if cfg.Postgres.DSN != "" {
		pool, err := pgxpool.New(ctx, cfg.Postgres.DSN)
		if err != nil {
			return backend{}, fmt.Errorf("connect to postgres: %w", err)
		}
		cleanups.add(func() error {
			pool.Close()
			return nil
		})
		if err := pool.Ping(ctx); err != nil {
			return backend{}, fmt.Errorf("ping postgres: %w", err)
		}

		repository := postgres.NewRepository(pool)
		if err := repository.Migrate(ctx, "./migrations"); err != nil {
			return backend{}, fmt.Errorf("migrate postgres: %w", err)
		}

		return backend{
			repository:   repository,
			healthPinger: repository,
		}, nil
	}

	storage := memstorage.NewMemStorage()
	snapshotStore, err := filestorage.NewStorage(cfg.SnapshotServiceConfig.FileStoragePath)
	if err != nil {
		return backend{}, fmt.Errorf("create snapshot storage: %w", err)
	}
	snapshotter := metrics.NewSnapshotCoordinator(
		snapshotStore,
		storage,
		cfg.SnapshotServiceConfig.StoreInterval,
	)
	if cfg.SnapshotServiceConfig.Restore {
		if err := snapshotter.Restore(ctx); err != nil {
			return backend{}, fmt.Errorf("restore snapshot: %w", err)
		}
	}

	return backend{
		repository:   storage,
		healthPinger: alwaysReadyPinger{},
		snapshotter:  snapshotter,
	}, nil
}
