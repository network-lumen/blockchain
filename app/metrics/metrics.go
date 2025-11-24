package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce  sync.Once
	pqcSignatures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lumen",
			Subsystem: "pqc",
			Name:      "signatures_total",
			Help:      "Count of PQC signature checks classified by result",
		},
		[]string{"result"},
	)

	pqcVerifySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "lumen",
			Subsystem: "pqc",
			Name:      "verify_seconds",
			Help:      "Time spent verifying Dilithium signatures",
			Buckets:   []float64{0.0005, 0.001, 0.002, 0.005, 0.01, 0.02, 0.05},
		},
	)
)

func ensureRegistered() {
	registerOnce.Do(func() {
		prometheus.MustRegister(pqcSignatures, pqcVerifySeconds)
	})
}

func SignaturesCounter() *prometheus.CounterVec {
	ensureRegistered()
	return pqcSignatures
}

func VerifyObserver() prometheus.Observer {
	ensureRegistered()
	return pqcVerifySeconds
}
