package main

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/glincker/levelrail/internal/agent"
	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/backup"
	"github.com/glincker/levelrail/internal/brand"
	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/cpbackup"
	"github.com/glincker/levelrail/internal/deploy"
	"github.com/glincker/levelrail/internal/deploylog"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/dockerguard"
	"github.com/glincker/levelrail/internal/email"
	"github.com/glincker/levelrail/internal/functions"
	ingressdriver "github.com/glincker/levelrail/internal/ingress"
	"github.com/glincker/levelrail/internal/integrity"
	"github.com/glincker/levelrail/internal/janitor"
	"github.com/glincker/levelrail/internal/loadbalancer"
	"github.com/glincker/levelrail/internal/migrateagent"
	"github.com/glincker/levelrail/internal/orphans"
	"github.com/glincker/levelrail/internal/plugin"
	"github.com/glincker/levelrail/internal/preview"
	"github.com/glincker/levelrail/internal/proxyroutes"
	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/reconcile/application"
	"github.com/glincker/levelrail/internal/safemode"
	"github.com/glincker/levelrail/internal/scheduledtask"
	"github.com/glincker/levelrail/internal/secrets"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/supplychain"
	"github.com/glincker/levelrail/internal/telemetry"
)

// bootState carries values shared between the ordered start stages of run.
type bootState struct {
	ctx                 context.Context
	stop                context.CancelFunc
	logger              *slog.Logger
	deferred            []func()
	pending             pendingStages
	deferredQueue       []bootStage
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
	buildClient         *build.Client
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
	livenessTracker     *application.LivenessTracker
}

// later registers a cleanup that runs after run's stages finish, in reverse order of registration.
func (s *bootState) later(fn func()) { s.deferred = append(s.deferred, fn) }

func (s *bootState) runDeferred() {
	for i := len(s.deferred) - 1; i >= 0; i-- {
		s.deferred[i]()
	}
}
