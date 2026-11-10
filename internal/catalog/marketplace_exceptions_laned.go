package catalog

// Namespace exceptions for templates whose project ships under a different registry org than its source org.
func init() {
	for id, why := range map[string]string{
		"ersatztv":    "the project publishes under the Docker Hub org jasongdove, while its source org is ErsatzTV",
		"kapowarr":    "the project publishes under the Docker Hub org mrcas, while its source org is Casvt",
		"lyrion":      "the project publishes under the Docker Hub org lmscommunity, while its source org is LMS-Community",
		"zwave-js-ui": "the project publishes under the Docker Hub org zwavejs, while its source org is zwave-js",
		"statping-ng": "the project publishes under the Docker Hub org adamboutcher, while its source org is statping-ng",
		"snappymail":  "the project publishes under the Docker Hub org djmaze, while its source org is the-djmaze",
		"profilarr":   "the project publishes under the Docker Hub org santiagosayshey, while its source org is Dictionarry-Hub",
		"evcc":        "the project publishes under the Docker Hub org evcc, while its source org is evcc-io",
		"stump":       "the project publishes under the Docker Hub org aaronleopold, while its source org is stumpapp",
	} {
		communityImageExceptions[id] = why
	}
	for id, why := range map[string]string{
		"domoticz": "upstream publishes calendar version tags with two parts only (2026.4)",
		"fenrus":   "upstream publishes calendar version tags with two parts only (26.10)",
		"nzbget":   "upstream publishes tags with two parts only (v26.3)",
	} {
		shortTagExceptions[id] = why
	}
}
