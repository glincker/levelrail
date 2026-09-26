package mcptools

import (
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Class is a tool's blast radius, the single source for its MCP
// annotations, the server modes, and the assistant's confirmation gate.
type Class int

const (
	// ClassUnset is the zero value of an unclassified tool.
	ClassUnset Class = iota
	// ClassRead tools change nothing.
	ClassRead
	// ClassMutate tools change state but can be undone or re-run.
	ClassMutate
	// ClassDestructive tools delete, roll back or tear down state.
	ClassDestructive
)

func (c Class) String() string {
	switch c {
	case ClassRead:
		return "read"
	case ClassMutate:
		return "mutate"
	case ClassDestructive:
		return "destructive"
	default:
		return "unclassified"
	}
}

type flag uint8

const (
	// flagSensitive marks a tool that touches credentials, secrets or
	// command output.
	flagSensitive flag = 1 << iota
	// flagOutbound marks a tool that reaches outside the control plane
	// (the MCP open-world hint).
	flagOutbound
	// flagUntrusted marks a tool whose result carries workload or
	// third-party authored text.
	flagUntrusted
)

// Meta is one tool's classification.
type Meta struct {
	Class Class
	Group string
	flags flag
}

// Sensitive reports whether the tool touches credentials, secrets or command output.
func (m Meta) Sensitive() bool { return m.flags&flagSensitive != 0 }

// Outbound reports whether the tool reaches outside the control plane.
func (m Meta) Outbound() bool { return m.flags&flagOutbound != 0 }

// Untrusted reports whether the tool's result carries attacker-influenced text.
func (m Meta) Untrusted() bool { return m.flags&flagUntrusted != 0 }

const (
	clsR = ClassRead
	clsM = ClassMutate
	clsD = ClassDestructive

	sens = flagSensitive
	outb = flagOutbound
	unt  = flagUntrusted

	// MetaSensitiveKey is the tool _meta key set on sensitive tools.
	MetaSensitiveKey = "levelrail/sensitive"
	// MetaUntrustedKey is the tool _meta key set on tools whose result carries untrusted text.
	MetaUntrustedKey = "levelrail/untrusted-output"
)

// toolTable classifies every registered tool. TestEveryToolClassified
// fails when a registered tool is missing here or an entry has no tool.
var toolTable = map[string]Meta{
	"list_apps":                             {clsR, "apps", 0},
	"get_app":                               {clsR, "apps", 0},
	"get_app_status":                        {clsR, "apps", unt},
	"get_app_timeline":                      {clsR, "apps", unt},
	"get_app_pending_changes":               {clsR, "apps", 0},
	"set_app_domains":                       {clsM, "domains", 0},
	"get_app_logs":                          {clsR, "logs", unt},
	"list_app_images":                       {clsR, "apps", 0},
	"list_deploys":                          {clsR, "deploys", unt},
	"list_deploy_attempts":                  {clsR, "deploys", unt},
	"list_failed_deploys":                   {clsR, "deploys", unt},
	"get_deploy_freeze":                     {clsR, "deploys", 0},
	"deploy_app":                            {clsM, "deploys", outb},
	"deploy_compose":                        {clsM, "deploys", outb},
	"rollback_app":                          {clsD, "deploys", 0},
	"cancel_deploy":                         {clsD, "deploys", 0},
	"restart_app":                           {clsM, "apps", 0},
	"clone_app":                             {clsM, "apps", 0},
	"bulk_apps":                             {clsM, "apps", 0},
	"bulk_delete_apps":                      {clsD, "apps", 0},
	"compare_deploys":                       {clsR, "deploys", unt},
	"promote_app":                           {clsM, "deploys", 0},
	"preview_promote_app":                   {clsR, "deploys", 0},
	"list_deploy_approvals":                 {clsR, "deploys", unt},
	"get_deploy_approval":                   {clsR, "deploys", unt},
	"approve_deploy_approval":               {clsM, "deploys", 0},
	"reject_deploy_approval":                {clsM, "deploys", 0},
	"get_app_git_source":                    {clsR, "apps", unt},
	"get_app_hook_runs":                     {clsR, "apps", sens | unt},
	"get_domain_tls_cert_status":            {clsR, "domains", 0},
	"list_databases":                        {clsR, "databases", 0},
	"get_database":                          {clsR, "databases", 0},
	"list_database_engines":                 {clsR, "databases", 0},
	"get_database_resource_recommendation":  {clsR, "metrics", 0},
	"get_resource_recommendation":           {clsR, "metrics", 0},
	"list_service_templates":                {clsR, "templates", 0},
	"get_service_template":                  {clsR, "templates", 0},
	"list_nodes":                            {clsR, "nodes", 0},
	"get_node":                              {clsR, "nodes", 0},
	"get_node_health":                       {clsR, "nodes", unt},
	"get_node_status_history":               {clsR, "nodes", 0},
	"list_preview_environments":             {clsR, "previews", unt},
	"sweep_stale_preview_environments":      {clsD, "previews", 0},
	"list_alert_rules":                      {clsR, "alerts", 0},
	"list_alert_silences":                   {clsR, "alerts", 0},
	"create_alert_silence":                  {clsM, "alerts", 0},
	"silence_alert_rule":                    {clsM, "alerts", 0},
	"expire_alert_silence":                  {clsM, "alerts", 0},
	"list_maintenance_windows":              {clsR, "alerts", 0},
	"list_alert_history":                    {clsR, "alerts", unt},
	"get_status_page":                       {clsR, "alerts", 0},
	"list_status_incidents":                 {clsR, "alerts", 0},
	"get_app_metrics":                       {clsR, "metrics", 0},
	"get_app_requests":                      {clsR, "metrics", 0},
	"diagnose_app_failure":                  {clsR, "diagnostics", unt},
	"get_attention":                         {clsR, "diagnostics", unt},
	"preflight_app":                         {clsR, "diagnostics", unt},
	"list_feature_flags":                    {clsR, "flags", 0},
	"get_feature_flag":                      {clsR, "flags", 0},
	"get_system_doctor":                     {clsR, "system", 0},
	"get_system_status":                     {clsR, "system", 0},
	"get_onboarding_status":                 {clsR, "system", 0},
	"prune_system":                          {clsD, "system", 0},
	"list_webhook_deliveries":               {clsR, "webhooks", unt},
	"list_backup_verifications":             {clsR, "backups", 0},
	"get_latest_backup_verification":        {clsR, "backups", 0},
	"list_backup_targets":                   {clsR, "backups", sens},
	"test_backup_target_connection":         {clsM, "backups", sens | outb},
	"list_control_plane_backups":            {clsR, "backups", 0},
	"create_control_plane_backup":           {clsM, "backups", sens},
	"verify_control_plane_backup":           {clsM, "backups", 0},
	"get_control_plane_dr_status":           {clsR, "backups", unt},
	"list_control_plane_offbox_backups":     {clsR, "backups", 0},
	"get_control_plane_drill_status":        {clsR, "backups", unt},
	"list_app_volume_backups":               {clsR, "backups", 0},
	"list_notification_channels":            {clsR, "notifications", sens},
	"list_notification_deliveries":          {clsR, "notifications", unt},
	"list_deploy_notify_targets":            {clsR, "notifications", sens},
	"list_audit_log":                        {clsR, "audit", unt},
	"list_iam_policies":                     {clsR, "iam", 0},
	"get_iam_policy":                        {clsR, "iam", 0},
	"list_organizations":                    {clsR, "orgs", 0},
	"list_projects":                         {clsR, "orgs", 0},
	"list_registry_credentials":             {clsR, "registry", sens},
	"list_registry_credential_repositories": {clsR, "registry", sens | outb},
	"list_registry_credential_tags":         {clsR, "registry", sens | outb},
	"list_environments":                     {clsR, "environments", 0},
	"preview_clone_environment":             {clsR, "environments", 0},
	"clone_environment":                     {clsM, "environments", 0},
	"list_domains":                          {clsR, "domains", 0},
	"get_app_network":                       {clsR, "domains", 0},
	"get_domain_maintenance_status":         {clsR, "domains", 0},
	"get_domain_redirect":                   {clsR, "domains", 0},
	"get_domain_error_pages":                {clsR, "domains", unt},
	"check_domain_dns":                      {clsR, "domains", outb},
	"plan_import":                           {clsR, "deploys", outb | unt},
	"list_certificates":                     {clsR, "domains", 0},
	"get_cloudflare_tunnel_status":          {clsR, "domains", outb},
	"get_app_log_drain":                     {clsR, "logs", sens},
	"list_storage_destinations":             {clsR, "logs", sens},
	"test_storage_destination":              {clsM, "logs", sens | outb},
	"list_log_archive_policies":             {clsR, "logs", 0},
	"set_log_archive_policy":                {clsM, "logs", outb},
	"start_log_archive_dump":                {clsM, "logs", outb},
	"list_log_archive_runs":                 {clsR, "logs", 0},
	"list_archived_logs":                    {clsR, "logs", unt},
	"get_build_cache":                       {clsR, "logs", 0},
	"set_build_cache":                       {clsM, "logs", outb},
	"get_oauth_providers":                   {clsR, "settings", sens},
	"get_email_settings":                    {clsR, "settings", sens},
	"list_scheduled_tasks":                  {clsR, "scheduled", 0},
	"get_scheduled_task":                    {clsR, "scheduled", 0},
	"list_models":                           {clsR, "models", 0},
	"get_model":                             {clsR, "models", 0},
	"list_gpu_nodes":                        {clsR, "models", 0},
	"list_model_keys":                       {clsR, "models", 0},
	"get_model_usage":                       {clsR, "models", 0},
	"get_model_logs":                        {clsR, "models", unt},
	"deploy_model":                          {clsM, "models", outb},
	"delete_model":                          {clsD, "models", 0},
	"restart_model":                         {clsM, "models", 0},
	"rotate_model_api_key":                  {clsM, "models", sens},
	"list_pipeline_runs":                    {clsR, "pipelines", unt},
	"list_all_pipeline_runs":                {clsR, "pipelines", unt},
	"explain_pipeline_run":                  {clsR, "pipelines", unt},
	"list_load_balancers":                   {clsR, "loadbalancer", 0},
	"get_app_load_balancer":                 {clsR, "loadbalancer", 0},
	"get_app_load_balancer_status":          {clsR, "loadbalancer", 0},
	"get_load_balancer_history":             {clsR, "loadbalancer", unt},
	"check_load_balancer":                   {clsM, "loadbalancer", 0},
	"set_load_balancer_upstream_state":      {clsM, "loadbalancer", 0},
	"set_app_load_balancer":                 {clsM, "loadbalancer", 0},
	"clear_app_load_balancer":               {clsD, "loadbalancer", 0},
	"export_app_load_balancer":              {clsR, "loadbalancer", 0},
	"plan_apply":                            {clsR, "iac", 0},
	"apply_resources":                       {clsD, "iac", 0},
}

// Lookup returns a tool's classification.
func Lookup(name string) (Meta, bool) {
	m, ok := toolTable[name]
	return m, ok
}

// Groups returns every toolset name, sorted.
func Groups() []string {
	seen := map[string]struct{}{}
	for _, m := range toolTable {
		seen[m.Group] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for g := range seen {
		names = append(names, g)
	}
	sort.Strings(names)
	return names
}

// annotationsFor treats an unclassified name as destructive and sensitive
// so it fails safe until TestEveryToolClassified forces a table entry.
func annotationsFor(name string) (*mcp.ToolAnnotations, map[string]any) {
	m, ok := toolTable[name]
	if !ok {
		m = Meta{Class: ClassDestructive, flags: sens | outb}
	}
	open := m.Outbound()
	a := &mcp.ToolAnnotations{
		Title:         titleFor(name),
		ReadOnlyHint:  m.Class == ClassRead,
		OpenWorldHint: &open,
	}
	if m.Class != ClassRead {
		destructive := m.Class == ClassDestructive
		a.DestructiveHint = &destructive
		a.IdempotentHint = strings.HasPrefix(name, "set_")
	}
	meta := map[string]any{}
	if m.Sensitive() {
		meta[MetaSensitiveKey] = true
	}
	if m.Untrusted() {
		meta[MetaUntrustedKey] = true
	}
	if len(meta) == 0 {
		meta = nil
	}
	return a, meta
}

func titleFor(name string) string {
	words := strings.Split(name, "_")
	words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	return strings.Join(words, " ")
}

// addTool registers a tool with annotations derived from toolTable.
func addTool[In, Out any](s *mcp.Server, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	t.Annotations, t.Meta = annotationsFor(t.Name)
	if m, ok := toolTable[t.Name]; ok && m.Untrusted() {
		h = wrapUntrustedResult(t.Name, h)
	}
	mcp.AddTool(s, t, h)
}
