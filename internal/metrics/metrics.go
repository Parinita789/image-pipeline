package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "httpRequestTotal",
			Help: "Total HTTP requests by method, route and status code",
		},
		[]string{"method", "route", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "httpRequestDurationSecond",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "route"},
	)

	// upload pipeline
	UploadEnqueuedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "uploadsEnqueuedTotal",
		Help: "Total images successfully enqueued for processing",
	})

	UploadErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "uploadErrorTotal",
			Help: "Total images successfully enqueued fro processing",
		},
		[]string{"reason"}, //s3 sqs, validation
	)

	ImageSizeBytes = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "imageSizeBytes",
		Help:    "Size of uploaded images in bytes",
		Buckets: []float64{1024, 102400, 512000, 1048576, 5242880, 10485760, 52428800},
	})

	// worker
	WorkerJobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workerJobsTotal",
			Help: "Total worker jobs processed by status",
		},
		[]string{"status"}, // completed, failed
	)

	WorkerJobDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "workerJobDurationSeconds",
		Help:    "Time taken to process a single job end to end",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120},
	})

	CompressionRatio = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "imageCompressionRatio",
		Help:    "Ratio of compressed to original size (lower is better)",
		Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
	})

	// Batch upload
	BatchUploadFilesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "batchUploadFilesTotal",
			Help: "Total files in batch uploads by status",
		},
		[]string{"status"},
	)

	// auth
	AuthRegistrationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "authRegistrationsTotal",
			Help: "Total registration attempts by status",
		},
		[]string{"status"},
	)

	AuthLoginsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "authLoginsTotal",
			Help: "Total login attempts by status",
		},
		[]string{"status"},
	)

	AuthLoginDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "authLoginDurationSeconds",
		Help:    "Login duration — bcrypt compare is the bottleneck",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1},
	})
)
