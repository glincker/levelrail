package runtimesettings

// Keys for the cloud doctor deep check, which makes one cheap read per enabled service.
const (
	KeyCloudDoctorDeepParallel    = "cloud.doctor.deep_parallel"
	KeyCloudDoctorDeepTimeoutSecs = "cloud.doctor.deep_timeout_seconds"
	KeyCloudDoctorDeepMinInterval = "cloud.doctor.deep_min_interval_seconds"
)

func init() {
	registerLimitInt(KeyCloudDoctorDeepParallel, "APP_CLOUD_DOCTOR_DEEP_PARALLEL", "4", "Cloud doctor deep check parallelism", "Most provider reads the deep check runs at once. From 1 to 16.", 1, 16)
	registerLimitInt(KeyCloudDoctorDeepTimeoutSecs, "APP_CLOUD_DOCTOR_DEEP_TIMEOUT_SECONDS", "15", "Cloud doctor deep check step timeout (seconds)", "Time one provider read may take before the step fails. From 2 to 120.", 2, 120)
	registerLimitInt(KeyCloudDoctorDeepMinInterval, "APP_CLOUD_DOCTOR_DEEP_MIN_INTERVAL_SECONDS", "10", "Cloud doctor deep check spacing (seconds)", "Least time between two deep checks of one account. From 0 to 3600.", 0, 3600)
}
