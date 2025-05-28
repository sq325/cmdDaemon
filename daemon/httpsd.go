package daemon

import (
	"encoding/json"
	"net/http"
)

// HttpSDResponse defines the structure for Prometheus HTTP service discovery.
// It is a slice of TargetGroup objects.
type HttpSDResponse []TargetGroup

// TargetGroup is a collection of targets that share a common set of labels.
type TargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func HttpSDHandler(d *Daemon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var response HttpSDResponse
		for _, dcmd := range d.DCmds {
			if dcmd.Annotations == nil {
				continue
			}
			if dcmd.Annotations[AnnotationsMetricsPathKey] == "" || dcmd.Annotations[AnnotationsIPKey] == "" || dcmd.Annotations[AnnotationsPortKey] == "" {
				continue // Skip commands that do not provide metrics
			}
			if dcmd.Status == Running {
				targets := []string{dcmd.Annotations[AnnotationsIPKey] + ":" + dcmd.Annotations[AnnotationsPortKey]}
				labels := map[string]string{
					AnnotationsNameKey:        dcmd.Annotations[AnnotationsNameKey],
					"hostAdmIp":               dcmd.Annotations[AnnotationsIPKey],
					AnnotationsMetricsPathKey: dcmd.Annotations[AnnotationsMetricsPathKey],
					AnnotationsHostnameKey:    dcmd.Annotations[AnnotationsHostnameKey],
					AnnotationsAppKey:         dcmd.Annotations[AnnotationsAppKey],
				}
				response = append(response, TargetGroup{Targets: targets, Labels: labels})
			}
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}
