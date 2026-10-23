package main

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/functions"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/integrity"
	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/migrateagent"
	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/plugin"
	"github.com/GLINCKER/levelrail/internal/preview"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/safemode"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// bootState carries values shared between the ordered start stages of run.
type bootState struct {
	ctx                 context.Context
	stop                context.CancelFunc
	logger              *slog.Logger
	deferred            []func()
	restartRequested    atomic.Bool
	ledger              *safemode.Ledger
	bootPolicy          safemode.Policy
	integrity           *integrity.Service
	db                  *store.DB
	instanceID          string
	guard               dockerguard.Booted
	client              *docker.Client
	b                   *brand.Brand
	telemetryDB         *telemetry.DB
	deployRecorder      *deploylog.Recorder
	logBroadcaster      *telemetry.LogBroadcaster
	alertingDB          *alerting.DB
	agentDataDir        string
	agentCA             *agent.CA
	agentRegistry       *agent.Registry
	meshEndpoints       *meshEndpointBook
	hostReports         *agent.HostReportStore
	onboardingReports   *agent.OnboardingStore
	agentServer         *agent.Server
	migrationAgent      *migrateagent.Service
	secretsManager      *secrets.Manager
	masterKeyFilePath   string
	emailSender         *email.DynamicSender
	notifyClient        *http.Client
	pushSender          alerting.PushSender
	pushVAPIDPublicKey  string
	deployDispatcher    *alerting.DeployDispatcher
	backupRunner        *backup.Runner
	backupVerifyRunner  *backup.VerifyRunner
	scheduledTaskRunner *scheduledtask.Runner
	functionQueue       *functions.Queue
	supplyChainSvc      *supplychain.Service
	builder             *deploy.Pipeline
	buildLimiter        *build.Limiter
	engine              *reconcile.Engine
	webhookHandler      http.Handler
	ingressDriver       *ingressdriver.Driver
	pluginMgr           *plugin.Manager
	apiRouter           *api.Router
	gateSecret          string
	cpDR                *cpbackup.Service
	httpServer          *http.Server
	lbRegistry          *loadbalancer.Registry
	meshCfg             *meshSetup
	meshDNSAddr         string
	previewManager      *preview.Manager
	orphanReaper        *orphans.Reaper
	janitorEngine       *janitor.Engine
	edge                ingressEdge
	proxySyncer         *proxyroutes.Syncer
	routeCheck          ingressdriver.RouteCheck
	gate                deployGate
	alertingEngine      *alerting.Engine
}

// later registers a cleanup that runs after run's stages finish, in reverse order of registration.
func (s *bootState) later(fn func()) { s.deferred = append(s.deferred, fn) }

func (s *bootState) runDeferred() {
	for i := len(s.deferred) - 1; i >= 0; i-- {
		s.deferred[i]()
	}
}
