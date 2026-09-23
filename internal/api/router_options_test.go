package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRouterOptions(t *testing.T) {
	t.Run("WithInviteTTL", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.inviteTTL)
		WithInviteTTL(5 * time.Minute)(rt)
		require.Equal(t, 5*time.Minute, rt.inviteTTL)
	})

	t.Run("WithSessionTTL", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.sessionTTL)
		WithSessionTTL(24 * time.Hour)(rt)
		require.Equal(t, 24*time.Hour, rt.sessionTTL)
	})

	t.Run("WithAutoPlacement", func(t *testing.T) {
		rt := &Router{}
		require.False(t, rt.autoPlacementEnabled)
		WithAutoPlacement(true)(rt)
		require.True(t, rt.autoPlacementEnabled)
	})

	t.Run("WithHSTS", func(t *testing.T) {
		rt := &Router{}
		require.False(t, rt.hstsEnabled)
		WithHSTS(true)(rt)
		require.True(t, rt.hstsEnabled)
	})

	t.Run("WithDataDir", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, "", rt.dataDir)
		WithDataDir("/var/lib/data")(rt)
		require.Equal(t, "/var/lib/data", rt.dataDir)
	})

	t.Run("WithDoctorMasterKeyRotationWarnAge", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.doctorMasterKeyRotationWarnAge)
		WithDoctorMasterKeyRotationWarnAge(90 * 24 * time.Hour)(rt)
		require.Equal(t, 90*24*time.Hour, rt.doctorMasterKeyRotationWarnAge)
	})

	t.Run("WithAgentCAFingerprint", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, "", rt.agentCAFingerprint)
		WithAgentCAFingerprint("fingerprint123")(rt)
		require.Equal(t, "fingerprint123", rt.agentCAFingerprint)
	})

	t.Run("WithDoctorIngressPorts", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, 0, rt.doctorHTTPPort)
		require.Equal(t, 0, rt.doctorHTTPSPort)
		WithDoctorIngressPorts(80, 443)(rt)
		require.Equal(t, 80, rt.doctorHTTPPort)
		require.Equal(t, 443, rt.doctorHTTPSPort)
	})

	t.Run("WithDoctorDiskWarningBytes", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, int64(0), rt.doctorDiskWarningBytes)
		WithDoctorDiskWarningBytes(1024)(rt)
		require.Equal(t, int64(1024), rt.doctorDiskWarningBytes)
	})

	t.Run("WithDoctorNetworkTimeout", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.doctorNetworkTimeout)
		WithDoctorNetworkTimeout(10 * time.Second)(rt)
		require.Equal(t, 10*time.Second, rt.doctorNetworkTimeout)
	})

	t.Run("WithDoctorPublicIPEndpoint", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, "", rt.doctorPublicIPEndpoint)
		WithDoctorPublicIPEndpoint("https://example.com/ip")(rt)
		require.Equal(t, "https://example.com/ip", rt.doctorPublicIPEndpoint)
	})

	t.Run("WithDoctorClockSkewWarnAge", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.doctorClockSkewWarnAge)
		WithDoctorClockSkewWarnAge(10 * time.Minute)(rt)
		require.Equal(t, 10*time.Minute, rt.doctorClockSkewWarnAge)
	})

	t.Run("WithDoctorMinRAMBytes", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, int64(0), rt.doctorMinRAMBytes)
		WithDoctorMinRAMBytes(1024)(rt)
		require.Equal(t, int64(1024), rt.doctorMinRAMBytes)
	})

	t.Run("WithDoctorMinCPUCount", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, 0, rt.doctorMinCPUCount)
		WithDoctorMinCPUCount(4)(rt)
		require.Equal(t, 4, rt.doctorMinCPUCount)
	})

	t.Run("WithCertExpiryWarningWindow", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.certExpiryWarningWindow)
		WithCertExpiryWarningWindow(14 * 24 * time.Hour)(rt)
		require.Equal(t, 14*24*time.Hour, rt.certExpiryWarningWindow)
	})

	t.Run("WithNodeAlertThresholds", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, float64(0), rt.nodeAlertThresholds.patchStatus)
		require.Equal(t, float64(0), rt.nodeAlertThresholds.nodeDiskSpace)
		require.Equal(t, float64(0), rt.nodeAlertThresholds.nodeCPU)
		require.Equal(t, float64(0), rt.nodeAlertThresholds.nodeMemory)
		WithNodeAlertThresholds(0.1, 0.2, 0.3, 0.4)(rt)
		require.Equal(t, float64(0.1), rt.nodeAlertThresholds.patchStatus)
		require.Equal(t, float64(0.2), rt.nodeAlertThresholds.nodeDiskSpace)
		require.Equal(t, float64(0.3), rt.nodeAlertThresholds.nodeCPU)
		require.Equal(t, float64(0.4), rt.nodeAlertThresholds.nodeMemory)
	})

	t.Run("WithPreviewTTL", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.previewTTL)
		WithPreviewTTL(24 * time.Hour)(rt)
		require.Equal(t, 24*time.Hour, rt.previewTTL)
	})

	t.Run("WithAuditLogRetention", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.auditLogRetention)
		WithAuditLogRetention(30 * 24 * time.Hour)(rt)
		require.Equal(t, 30*24*time.Hour, rt.auditLogRetention)
	})

	t.Run("WithSecretRotationWarnAge", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.secretRotationWarnAge)
		WithSecretRotationWarnAge(60 * 24 * time.Hour)(rt)
		require.Equal(t, 60*24*time.Hour, rt.secretRotationWarnAge)
	})

	t.Run("WithDeployApprovalTTL", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.deployApprovalTTL)
		WithDeployApprovalTTL(2 * time.Hour)(rt)
		require.Equal(t, 2*time.Hour, rt.deployApprovalTTL)
	})

	t.Run("WithResourceRecommendationLookback", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, time.Duration(0), rt.resourceRecommendationLookback)
		WithResourceRecommendationLookback(7 * 24 * time.Hour)(rt)
		require.Equal(t, 7*24*time.Hour, rt.resourceRecommendationLookback)
	})

	t.Run("WithPublicHost", func(t *testing.T) {
		rt := &Router{}
		require.Equal(t, "", rt.publicHost)
		WithPublicHost("dashboard.example.com")(rt)
		require.Equal(t, "dashboard.example.com", rt.publicHost)
	})

	t.Run("WithAllowInsecureLogin", func(t *testing.T) {
		rt := &Router{}
		require.False(t, rt.allowInsecureLogin)
		WithAllowInsecureLogin(true)(rt)
		require.True(t, rt.allowInsecureLogin)
	})

	t.Run("WithAPIRateLimit", func(t *testing.T) {
		rt := &Router{}
		require.Nil(t, rt.apiRateLimit)
		WithAPIRateLimit(100, 50)(rt)
		require.NotNil(t, rt.apiRateLimit)
	})

	t.Run("WithWebhookRateLimit", func(t *testing.T) {
		rt := &Router{}
		require.Nil(t, rt.webhookRateLimit)
		WithWebhookRateLimit(20)(rt)
		require.NotNil(t, rt.webhookRateLimit)
	})
}
